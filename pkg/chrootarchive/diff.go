package chrootarchive

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/reexec"
)

type applyLayerResponse struct {
	LayerSize int64 `json:"layerSize"`
}

func applyLayer() {
	runtime.LockOSThread()
	flag.Parse()

	var options *archive.TarOptions

	if err := json.Unmarshal([]byte(os.Getenv("OPT")), &options); err != nil {
		fatal(err)
	}

	if err := chroot(flag.Arg(0)); err != nil {
		fatal(err)
	}

	// We need to be able to set any perms
	oldmask := syscall.Umask(0)
	defer syscall.Umask(oldmask)
	tmpDir, err := ioutil.TempDir("/", "temp-docker-extract")
	if err != nil {
		fatal(err)
	}

	os.Setenv("TMPDIR", tmpDir)
	size, err := archive.UnpackLayer("/", os.Stdin, options)
	os.RemoveAll(tmpDir)
	if err != nil {
		fatal(err)
	}

	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(applyLayerResponse{size}); err != nil {
		fatal(fmt.Errorf("unable to encode layerSize JSON: %s", err))
	}

	flush(os.Stdout)
	flush(os.Stdin)
	os.Exit(0)
}

func ApplyLayer(dest string, layer archive.ArchiveReader, options *archive.TarOptions) (size int64, err error) {
	dest = filepath.Clean(dest)
	decompressed, err := archive.DecompressStream(layer)
	if err != nil {
		return 0, err
	}

	defer decompressed.Close()
	if options == nil {
		options = &archive.TarOptions{}
	}
	if options.ExcludePatterns == nil {
		options.ExcludePatterns = []string{}
	}

	data, err := json.Marshal(options)
	if err != nil {
		return 0, fmt.Errorf("ApplyLayer json encode: %v", err)
	}
	cmd := reexec.Command("docker-applyLayer", dest)
	cmd.Stdin = decompressed
	cmd.Env = append(cmd.Env, fmt.Sprintf("OPT=%s", data))

	outBuf, errBuf := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = outBuf, errBuf

	if err = cmd.Run(); err != nil {
		return 0, fmt.Errorf("ApplyLayer %s stdout: %s stderr: %s", err, outBuf, errBuf)
	}

	// Stdout should be a valid JSON struct representing an applyLayerResponse.
	response := applyLayerResponse{}
	decoder := json.NewDecoder(outBuf)
	if err = decoder.Decode(&response); err != nil {
		return 0, fmt.Errorf("unable to decode ApplyLayer JSON response: %s", err)
	}

	return response.LayerSize, nil
}
