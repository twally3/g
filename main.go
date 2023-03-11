package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

type OperatingSystem string

const (
	Darwin  OperatingSystem = "darwin"
	Linux   OperatingSystem = "linux"
	Windows OperatingSystem = "windows"
	Freebsd OperatingSystem = "freebsd"
)

type Extension string

const (
	Tar Extension = ".tar.gz"
	Zip Extension = ".zip"
)

type Architecture string

const (
	Amd Architecture = "amd64"
	Arm Architecture = "arm64"
)

type Shell string

const (
	Zsh  Shell = ".zshrc"
	Bash Shell = ".bash_profile"
)

type System struct {
	Architecture Architecture
	Extension    Extension
	OS           OperatingSystem
	Shell        Shell
}

func getArch() (Architecture, error) {
	switch runtime.GOARCH {
	case "amd64":
		return Amd, nil
	case "arm64":
		return Arm, nil
	default:
		return "", errors.New("unsupported goarch")
	}
}

func getShell() (Shell, error) {
	shell := strings.Split(os.Getenv("SHELL"), "/")[len(strings.Split(os.Getenv("SHELL"), "/"))-1]
	switch shell {
	case "bash":
		return Bash, nil
	case "zsh":
		return Zsh, nil
	default:
		return "", fmt.Errorf("unknown shell %s", shell)
	}
}
func getSystem() (*System, error) {
	arch, err := getArch()
	if err != nil {
		return nil, err
	}

	shell, err := getShell()
	if err != nil {
		return nil, err
	}

	switch runtime.GOOS {
	case "darwin":
		return &System{arch, Tar, Darwin, shell}, nil
	case "linux":
		return &System{arch, Tar, Linux, shell}, nil
	// case "windows":
	// 	return &System{Amd, Zip, Windows}, nil
	default:
		return nil, errors.New("unsupported GOOS")
	}
}

func DownloadFile(filepath string, url string) error {
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Send the GET request
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Write the response to the file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func untargz(inpath string, outpath string) error {
	file, err := os.Open(inpath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzf, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzf.Close()

	// create a new tar tr
	tr := tar.NewReader(gzf)

	// iterate over each file in the tar archive
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Extract the file to the temporary directory
		target := filepath.Join(outpath, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			// Create the directory if it doesn't exist
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			// Create the file and write the contents
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(file, tr); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unable to extract file %s of type %c", header.Name, header.Typeflag)
		}
	}
	return nil
}

func writePath(profilePath string, binPath string) error {
	exportCmd := fmt.Sprintf("export PATH=%s:$PATH", binPath)

	file, err := os.OpenFile(profilePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, exportCmd) {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	_, err = file.WriteString(fmt.Sprintf("%s\n", exportCmd))
	if err != nil {
		return err
	}

	return nil
}

func main() {
	currentUser, err := user.Current()
	if err != nil {
		panic(err)
	}

	groot := filepath.Join(currentUser.HomeDir, ".g")

	if len(os.Args) == 1 {
		fmt.Println("You need help")
		return
	}

	if len(os.Args) > 2 {
		fmt.Println("SHEEEEEET")
		return
	}

	system, err := getSystem()
	if err != nil {
		panic(err)
	}

	version := os.Args[1]

	if _, err := os.Stat(groot); os.IsNotExist(err) {
		err := os.Mkdir(groot, 0755)
		if err != nil {
			panic(err)
		}
	}

	pkg := fmt.Sprintf("go%s.%s-%s%s", version, system.OS, system.Architecture, system.Extension)
	dlroot := "https://go.dev/dl/"
	url := fmt.Sprintf("%s%s", dlroot, pkg)
	pkgDir := filepath.Join(groot, pkg)
	versionRoot := filepath.Join(groot, version)

	if err := DownloadFile(pkgDir, url); err != nil {
		panic(err)
	}

	if err := untargz(pkgDir, versionRoot); err != nil {
		panic(err)
	}

	lnsrc := filepath.Join(versionRoot, "go")
	lndest := filepath.Join(groot, "go")
	fmt.Printf("Updating symlink %s => %s\n", lnsrc, lndest)

	err = os.RemoveAll(lndest)
	if err != nil {
		panic(err)
	}

	if err := os.Symlink(lnsrc, lndest); err != nil {
		panic(err)
	}

	profilePath := filepath.Join(currentUser.HomeDir, string(system.Shell))
	binPath := filepath.Join(lndest, "bin")

	if err := writePath(profilePath, binPath); err != nil {
		panic(err)
	}

	fmt.Println("DONE")
}
