package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"time"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

// Database struct contains connection credentials
type Database struct {
	Name     string
	Host     string
	Port     string
	Username string
	Password string
}

// DumpFile describes database dump file
type DumpFile struct {
	Name      string
	Separator string
	Suffix    string
	Prefix    string
	Extension string
	Directory string
}

// Author struct contains git author info
type Author struct {
	Name  string
	Email string
}

// Repository represents git repository
type Repository struct {
	url.URL
	Directory      string
	Author         Author
	PrivateKeyPath string
}

const repositoryDirectory = "repository"
const repositoryBackupFile = "dump.sql"
const mysqlDefaultPort = "3306"
const dumpFilePrefix = "dump"
const dumpFileExt = "sql"

var (
	gitCmd = flag.NewFlagSet("git", flag.ExitOnError)

	configFile     = flag.String("wpConfig", "/var/www/wordpress/wp-config.php", "path to wordpress configuration file (wp-config.php)")
	outputDir      = flag.String("outputDir", "/backups", "output directory to store database dump")
	repositoryURL  = gitCmd.String("repositoryUrl", "", "Git repository url (f.ex. git@gitlab.com:username/repository-name.git). (Required)")
	privateKeyPath = gitCmd.String("privateKeyPath", "", "Private key path for git login via ssh. (Required)")
	authorName     = gitCmd.String("authorName", "", "Git commit author name. (Required)")
	authorEmail    = gitCmd.String("authorEmail", "", "Git commit author email. (Required)")
)

func init() {
	// Parse the flags for appropriate FlagSet
	// FlagSet.Parse() requires a set of arguments to parse as input
	// os.Args[i+1:] will be all arguments starting after the subcommand at os.Args[1]
	i := getValueKey(os.Args, "git")

	if len(os.Args) > 1 && i > -1 {
		// required flags for git
		gitCmd.Parse(os.Args[i+1:])

		if *repositoryURL == "" || *privateKeyPath == "" || *authorName == "" || *authorEmail == "" {
			gitCmd.PrintDefaults()
			os.Exit(1)
		}
	}

	flag.Parse()
}

func main() {
	config := parseWpConfig(*configFile)

	db := Database{
		Name:     config["DB_NAME"],
		Host:     config["DB_HOST"],
		Port:     config["DB_PORT"],
		Username: config["DB_USER"],
		Password: config["DB_PASSWORD"],
	}

	outputFile := DumpFile{
		Separator: "-",
		Prefix:    dumpFilePrefix,
		Extension: dumpFileExt,
		Directory: *outputDir,
	}

	//outputFile.Name = "dump"

	dumpDatabase(db, &outputFile)

	if gitCmd.Parsed() {
		repository, err := parse(*repositoryURL)
		checkError(err)

		author := Author{
			Name:  *authorName,
			Email: *authorEmail,
		}

		repository.Directory = repositoryDirectory
		repository.Author = author
		repository.PrivateKeyPath = checkUserPrivateKey(*privateKeyPath)

		pushChanges(repository, repositoryBackupFile, outputFile.getPathName())
	}
}

func parseWpConfig(configFile string) map[string]string {
	config := make(map[string]string)
	patterns := []string{"DB_NAME", "DB_USER", "DB_PASSWORD", "DB_HOST", "DB_PORT"}

	// Open file and create scanner on top of it
	file, err := os.Open(configFile)
	checkError(err)

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		// Get data from scan with Bytes() or Text()
		line := scanner.Text()
		if strings.Contains(line, "DB_") {
			for _, pattern := range patterns {
				r := regexp.MustCompile(`(?m)^define\(\s*'` + pattern + `'\s*,\s*'(.+)'\s*\);`)
				match := r.FindStringSubmatch(line)
				if len(match) > 0 {
					config[pattern] = match[1]
				}
			}
		}
	}

	return config
}

func dumpDatabase(db Database, outputFile *DumpFile) {
	if db.Port == "" {
		db.Port = mysqlDefaultPort
	}
	// dump date
	t := time.Now()
	outputFile.Suffix = t.Format("20060102150405")

	cmd := exec.Command("mysqldump", "--host", db.Host, "--port", db.Port, "--user", db.Username, "--password="+db.Password, db.Name)

	stdout, err := cmd.StdoutPipe()
	checkError(err)

	err = cmd.Start()
	checkError(err)

	bytes, err := ioutil.ReadAll(stdout)
	checkError(err)

	if len(bytes) == 0 {
		log.Fatal(errors.New("couldn't dump a database, check the connection"))
	}

	f, err := os.Create(outputFile.getPathName())
	checkError(err)

	defer f.Close()

	c, err := f.Write(bytes)
	checkError(err)

	fmt.Println("Wrote", c, "bytes")
}

// clone git repository if does not exists and pull latest changes, create a commit and push changes
// url is the repo url in format: git@gitlab.com:username/project.git
// directory is a working directory for the repository
// file is the path inside working directory which will be commited
// author is a commit author
// source file is current dump file path (outside repository) which should be copied to repository path and pushed to the repository
func pushChanges(repository *Repository, file string, src string) {
	// it's required to add an entry to known_hosts
	// ssh-keyscan -H gitlab.com >> ~/.ssh/known_hosts
	addHostToKnownHosts(repository.Host)

	sshAuth, err := ssh.NewPublicKeysFromFile("git", repository.PrivateKeyPath, "")
	checkError(err)

	var r *git.Repository

	// clone repository if does not exist
	if _, err := os.Stat(repository.Directory + "/.git"); os.IsNotExist(err) {
		r, err = git.PlainClone(repository.Directory, false, &git.CloneOptions{
			URL:               repository.String(),
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
			Auth:              sshAuth,
			Progress:          os.Stdout,
		})

		if err != nil && err.Error() != "remote repository is empty" {
			checkError(err)
		}
	} else if isEmptyDir(repository.Directory + "/.git/refs/heads") {
		log.Println("Repository is empty")

	} else {
		r, err = git.PlainOpen(repository.Directory)
		checkError(err)
	}

	// Get the working directory for the repository
	w, err := r.Worktree()
	checkError(err)

	// Pull the latest changes from the origin remote and merge into the current branch
	// If already up-to-date it will generate error, so we don't check it
	w.Pull(&git.PullOptions{RemoteName: "origin"})
	checkError(err)

	// copies our dump file to the repository directory as a new name
	dest := pwd() + string(os.PathSeparator) + strings.Trim(repository.Directory, string(os.PathSeparator)) + string(os.PathSeparator) + file

	copyFile(src, dest)

	// add file
	_, err = w.Add(file)
	checkError(err)

	// check status
	status, err := w.Status()
	checkError(err)

	fmt.Println(status)

	// prepare commit
	commit, err := w.Commit("example commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  repository.Author.Name,
			Email: repository.Author.Email,
			When:  time.Now(),
		},
	})
	checkError(err)

	// Print current HEAD
	obj, err := r.CommitObject(commit)
	checkError(err)

	fmt.Println(obj)

	// push using default options
	err = r.Push(&git.PushOptions{
		Auth:     sshAuth,
		Progress: os.Stdout,
	})
	checkError(err)
}

func isEmptyDir(name string) bool {
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true
	}

	return false // Either not empty or error, suits both cases
}

func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// copyFile copies a file from source to destination
func copyFile(src string, dest string) {
	from, err := os.Open(src)
	checkError(err)
	defer from.Close()

	to, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE, 0666)
	checkError(err)
	defer to.Close()

	_, err = io.Copy(to, from)
	checkError(err)
}

func (df *DumpFile) getName() string {
	if df.Name == "" {
		if df.Prefix == "" && df.Suffix == "" {
			return ""
		}

		if df.Prefix == "" {
			df.Name = df.Suffix + "." + df.Extension
		} else if df.Suffix == "" {
			df.Name = df.Prefix + "." + df.Extension
		} else {
			df.Name = df.Prefix + df.Separator + df.Suffix + "." + df.Extension
		}
	}

	return df.Name
}

func (df *DumpFile) getPathName() string {
	fileName := df.getName()
	if fileName == "" || df.Directory == "" {
		return ""
	}

	return strings.TrimSuffix(df.Directory, string(os.PathSeparator)) + string(os.PathSeparator) + fileName
}

func pwd() string {
	pwd, err := os.Getwd()
	checkError(err)

	return pwd
}

func parse(rawurl string) (*Repository, error) {
	var u Repository
	var m *url.URL

	// check using standard URL lib (f.ex. for github)
	m, err := url.Parse(rawurl)

	if m != nil {
		return &(Repository{URL: *m}), nil
	}

	// patern for gitlab
	// scp format: your_username@remotehost.edu:foobar.txt
	var pattern = `(?m)^([^@]+)@{1}([^:]+):/?(.+)`
	match, err := regexp.MatchString(pattern, rawurl)

	if err != nil || match == false {
		return nil, errors.New("Incorrect repository url")
	}

	var r = regexp.MustCompile(pattern)
	groups := r.FindStringSubmatch(rawurl)

	u.User = url.User(groups[1])
	u.Host = groups[2]
	u.Path = groups[3]

	return &u, nil
}

func (r *Repository) String() string {
	u := r.URL
	if u.Scheme == "" && u.User != nil && u.Host != "" { //scp format
		var buf bytes.Buffer

		if ui := u.User; ui != nil {
			buf.WriteString(ui.String())
			buf.WriteByte('@')
		}
		if h := u.Host; h != "" {
			buf.WriteString(h)
		}

		path := u.EscapedPath()
		if path != "" && path[0] != '/' && u.Host != "" {
			buf.WriteByte(':')
		}

		buf.WriteString(path)

		return buf.String()
	}

	return u.String()
}

// returns path to private key from homedir
func checkUserPrivateKey(p string) string {
	if _, err := os.Stat(p); err == nil {
		return p
	}

	// return default
	currentUser, err := user.Current()
	checkError(err)

	return currentUser.HomeDir + "/.ssh/id_rsa"
}

func addHostToKnownHosts(host string) string {
	// @TODO: omit if host is already in known_hosts
	cmd := exec.Command("sh", "-c", "ssh-keyscan "+host+" >> ~/.ssh/known_hosts")
	out, err := cmd.Output()

	checkError(err)

	return string(out)

}

// check if a slice contains given string and returns its index or -1
func getValueKey(s []string, c string) int {
	for k, v := range s {
		if v == c {
			return k
		}
	}
	return -1
}
