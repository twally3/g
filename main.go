package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
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

type ProgressBar struct {
	current int64
	total   int64
}

// https://darkcoding.net/software/pretty-command-line-console-output-on-unix-in-python-and-go-lang/
// TODO: Change this to work more like the above
func (p *ProgressBar) Write(b []byte) (int, error) {
	n := len(b)
	p.current += int64(n)
	if p.total == 0 {
		// TODO: Find out what happens if you return error
		fmt.Println("Unknown progress...")
		return n, nil
	}
	percent := float32(p.current) / float32(p.total) * 100
	fmt.Printf("\rDownloading... %.2f%%", percent)
	return n, nil
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

	progressBar := &ProgressBar{}
	progressBar.total = resp.ContentLength
	reader := io.TeeReader(resp.Body, progressBar)

	// Write the response to the file
	_, err = io.Copy(out, reader)
	if err != nil {
		return err
	}

	fmt.Println("")

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

type GoVersion struct {
	Version  string `json:"version"`
	Stable   bool   `json:"stable"`
	archived bool
	Semver   string
}

func makeGoVersionsRequest(includeArchived bool) ([]GoVersion, error) {
	url := "https://golang.org/dl/?mode=json"
	if includeArchived {
		url = url + "&include=all"
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var versions []GoVersion
	err = json.NewDecoder(resp.Body).Decode(&versions)
	if err != nil {
		return nil, err
	}

	return versions, nil
}

func getGoVersions() ([]GoVersion, error) {
	currentVersions, err := makeGoVersionsRequest(false)
	if err != nil {
		return nil, err
	}

	versionSet := map[string]bool{}
	for _, v := range currentVersions {
		versionSet[v.Version] = true
	}

	allVersions, err := makeGoVersionsRequest(true)
	if err != nil {
		return nil, err
	}

	finalVersions := []GoVersion{}

	for _, v := range allVersions {
		if _, ok := versionSet[v.Version]; !ok {
			v.archived = true
		}

		v.Semver = calculateSemver(v.Version)

		finalVersions = append(finalVersions, v)
	}

	return finalVersions, nil
}

func calculateSemver(versionString string) string {
	re := regexp.MustCompile(`^go(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:(\w+)(\d+))?$`)

	match := re.FindStringSubmatch(versionString)

	if match == nil {
		panic("Failed to match go version")
	}

	major := match[1]
	if major == "" {
		major = "0"
	}
	minor := match[2]
	if minor == "" {
		minor = "0"
	}
	patch := match[3]
	if patch == "" {
		patch = "0"
	}

	distTag := match[4]
	build := match[5]

	semver := fmt.Sprintf("%s.%s.%s", major, minor, patch)
	preRelease := distTag
	if build != "" {
		preRelease = fmt.Sprintf("%s.%s", preRelease, build)
	}

	if preRelease != "" {
		semver = fmt.Sprintf("%s-%s", semver, preRelease)
	}

	return semver
}

func deleteFiles(glob string) error {
	files, err := filepath.Glob(glob)
	if err != nil {
		return err
	}

	for _, file := range files {
		err := os.Remove(file)
		if err != nil {
			fmt.Printf("Failed to delete %s\n", file)
		}
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

	fmt.Printf("Searching for a version that matches %s\n", version)
	versions, err := getGoVersions()
	if err != nil {
		panic(err)
	}

	idx := -1
	for i, v := range versions {
		if (version == "stable" || version == "lts") && v.Stable {
			idx = i
			break
		}
		if strings.HasPrefix(v.Semver, version) {
			idx = i
			break
		}
	}

	if idx == -1 {
		panic("Could not satisfy version")
	}

	goVersion := versions[idx]
	fmt.Printf("Found matching go version %s\n", goVersion.Semver)

	if _, err := os.Stat(groot); os.IsNotExist(err) {
		err := os.Mkdir(groot, 0755)
		if err != nil {
			panic(err)
		}
	}

	pkg := fmt.Sprintf("%s.%s-%s%s", goVersion.Version, system.OS, system.Architecture, system.Extension)
	dlroot := "https://go.dev/dl/"
	url := fmt.Sprintf("%s%s", dlroot, pkg)
	pkgDir := filepath.Join(groot, pkg)
	versionRoot := filepath.Join(groot, goVersion.Semver)

	if _, err := os.Stat(versionRoot); os.IsNotExist(err) {
		fmt.Println("Version is not local, downloading...")
		if err := DownloadFile(pkgDir, url); err != nil {
			panic(err)
		}

		if err := untargz(pkgDir, versionRoot); err != nil {
			panic(err)
		}

		if err := deleteFiles(filepath.Join(groot, "*.tar.gz")); err != nil {
			panic(err)
		}
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
