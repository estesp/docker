// +build experimental

package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/system"
	"github.com/go-check/check"
)

func (s *DockerSuite) TestExperimentalVersion(c *check.C) {
	out, _ := dockerCmd(c, "version")
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Experimental (client):") || strings.HasPrefix(line, "Experimental (server):") {
			c.Assert(line, check.Matches, "*true")
		}
	}

	out, _ = dockerCmd(c, "-v")
	if !strings.Contains(out, ", experimental") {
		c.Fatalf("docker version did not contain experimental: %s", out)
	}
}

// user namespaces test: run daemon with remapped root setting
// 1. validate uid/gid maps are set properly
// 2. verify that files created are owned by remapped root
func (s *DockerDaemonSuite) TestDaemonUserNamespaceRootSetting(c *check.C) {
	testRequires(c, NativeExecDriver)
	testRequires(c, SameHostDaemon)

	c.Assert(s.d.StartWithBusybox("--root", "9999:9999"), check.IsNil)

	tmpDir, err := ioutil.TempDir("", "userns")
	if err != nil {
		c.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	//writeable by the remapped root UID/GID pair
	c.Assert(os.Chown(tmpDir, 9999, 9999), check.IsNil)

	out, err := s.d.Cmd("run", "-d", "--name", "userns9999", "-v", tmpDir+":/goofy", "busybox", "touch", "/goofy/testfile", ";", "top")
	c.Assert(err, check.IsNil, check.Commentf("Output: %s", out))

	pid, err := s.d.Cmd("inspect", "--format='{{.State.Pid}}'", "userns9999")
	if err != nil {
		c.Fatalf("Could not inspect running container: out: %q; err: %v", pid, err)
	}
	// check the uid and gid maps for the PID to ensure root is remapped
	// (cmd = cat /proc/<pid>/uid_map | grep -E '0\s+9999\s+1')
	out, rc1, err := runCommandPipelineWithOutput(
		exec.Command("cat", "/proc/"+strings.TrimSpace(pid)+"/uid_map"),
		exec.Command("grep", "-E", "0[[:space:]]+9999[[:space:]]+1"))
	c.Assert(rc1, check.Equals, 0, check.Commentf("Didn't match uid_map: output: %s", out))

	out, rc2, err := runCommandPipelineWithOutput(
		exec.Command("cat", "/proc/"+strings.TrimSpace(pid)+"/gid_map"),
		exec.Command("grep", "-E", "0[[:space:]]+9999[[:space:]]+1"))
	c.Assert(rc2, check.Equals, 0, check.Commentf("Didn't match gid_map: output: %s", out))

	// check that the touched file is owned by 9999:9999
	stat, err := system.Stat(filepath.Join(tmpDir, "testfile"))
	if err != nil {
		c.Fatal(err)
	}
	c.Assert(stat.Uid(), check.Equals, uint32(9999), check.Commentf("Touched file not owned by remapped root UID"))
	c.Assert(stat.Gid(), check.Equals, uint32(9999), check.Commentf("Touched file not owned by remapped root GID"))
}
