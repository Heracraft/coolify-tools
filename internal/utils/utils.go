package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"encoding/json"

	"github.com/Heracraft/coolify-tools/internal/docker"
)

func HandleErr(format string, err error, args ...any) {
	if err != nil {
		log.Fatalf(format+": %v", append(args, err)...)
	}
}

func ParseOutputAsNumber(stdout []byte) int {
	cleanStr := strings.TrimSpace(string(stdout))

	number, err := strconv.Atoi(cleanStr)

	HandleErr("failed to convert output to number", err)

	return number
}

func Exists(dir string) bool {
	// checks if dir/file exists
	_, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return true
}

// cred: https://dev.to/tidalcloud/interactive-cli-prompts-in-go-3bj9
func YesNoPrompt(label string, def bool) bool {
	choices := "Y/n"
	if !def {
		choices = "y/N"
	}

	r := bufio.NewReader(os.Stdin)
	var s string

	for {
		fmt.Fprintf(os.Stderr, "%s (%s) ", label, choices)
		s, _ = r.ReadString('\n')
		s = strings.TrimSpace(s)
		if s == "" {
			return def
		}
		s = strings.ToLower(s)
		if s == "y" || s == "yes" {
			return true
		}
		if s == "n" || s == "no" {
			return false
		}
	}
}

// ---

func GetCoolifyVersion(client *ssh.Client) string {
	session, err := client.NewSession()
	HandleErr("Failed to create ssh session", err)
	defer session.Close()

	output, err := session.Output("docker inspect coolify")
	HandleErr("failed to inspect coolify", err)

	var coolifyContainers []docker.Container

	if err := json.Unmarshal(output, &coolifyContainers); err != nil {
		HandleErr("Failed to parse docker inspect output", err)
	}

	if len(coolifyContainers) == 9 {
		return "unknown"
	}

	version := strings.Split(coolifyContainers[0].Config.Image, "coolify:")

	return version[1]
}

func InstallCoolify(client *ssh.Client, version string) {
	cmdStr := "curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash -s " + version

	session, err := client.NewSession()
	HandleErr("Failed to create ssh session", err)
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	fmt.Println("Starting coolify installation v" + version)

	err = session.Run(cmdStr)

	if stdout.Len() > 0 {
		fmt.Printf("Output:\n%s\n", stdout.String())
	}

	if err != nil {
		fmt.Printf("Execution Error: %v\n", err)
		if stderr.Len() > 0 {
			fmt.Printf("Stderr Output:\n%s\n", stderr.String())
		}
		return
	}

	fmt.Println("Installation command completed successfully.")
}
