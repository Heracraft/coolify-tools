package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"

	"coolify-tools/internal/utils"
)

func GetSigner(key string, flagPassphrase string) ssh.Signer {
	file, err := os.ReadFile(os.Getenv("HOME") + "/.ssh/" + key)

	utils.HandleErr("Failed to read ssh key", err)

	var privateKey ssh.Signer

	privateKey, err = ssh.ParsePrivateKey(file)

	var PassphraseMissingError *ssh.PassphraseMissingError

	if err != nil {
		if errors.As(err, &PassphraseMissingError) {
			var passphrase []byte

			if flagPassphrase == "" {

				fmt.Print("Enter key passphrase: ")
				var pErr error
				passphrase, pErr = term.ReadPassword(int(syscall.Stdin))
				utils.HandleErr("failed to read password", pErr)

				defer func() {
					for i := range passphrase {
						passphrase[i] = 0
					}
				}()
			} else {
				passphrase = []byte(flagPassphrase)

				defer func() {
					for i := range passphrase {
						passphrase[i] = 0
					}
				}()
			}

			privateKey, err = ssh.ParsePrivateKeyWithPassphrase(file, passphrase)

			utils.HandleErr("failed to parse private key with passphrase", err)

			return privateKey

		}

		log.Fatalf("Failed to parse private key: %v", err)
	}

	return privateKey
}

func EstablishConnection(username string, hostname string, privateKey string, sshPort string, flagPassphrase string) (*ssh.Client, ssh.Signer) {
	signer := GetSigner(privateKey, flagPassphrase)

	hostCallback := CreateInteractiveHostKeyCallback(os.Getenv("HOME") + "/.ssh/known_hosts")

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(
				signer,
			),
		},
		HostKeyCallback: hostCallback,
	}

	remoteAddr := net.JoinHostPort(hostname, sshPort)

	client, err := ssh.Dial("tcp", remoteAddr, config)

	if err != nil {
		log.Fatalf("Failed to establish ssh connection: %v", err)
	}

	return client, signer
}

func GetRawPrivateKey(key string, flagPassphrase string) interface{} {
	file, err := os.ReadFile(os.Getenv("HOME") + "/.ssh/" + key)

	utils.HandleErr("Failed to read ssh key", err)

	// var privateKey ssh.Signer

	privateKey, err := ssh.ParseRawPrivateKey(file)

	var PassphraseMissingError *ssh.PassphraseMissingError

	if err != nil {
		if errors.As(err, &PassphraseMissingError) {
			var passphrase []byte

			if flagPassphrase == "" {

				fmt.Print("Enter key passphrase: ")
				var pErr error
				passphrase, pErr = term.ReadPassword(int(syscall.Stdin))
				utils.HandleErr("failed to read password", pErr)

				defer func() {
					for i := range passphrase {
						passphrase[i] = 0
					}
				}()
			} else {
				passphrase = []byte(flagPassphrase)

				defer func() {
					for i := range passphrase {
						passphrase[i] = 0
					}
				}()
			}

			privateKey, err = ssh.ParseRawPrivateKeyWithPassphrase(file, passphrase)

			utils.HandleErr("failed to parse private key with passphrase", err)

			return privateKey

		}

		log.Fatalf("Failed to parse private key: %v", err)
	}

	return privateKey
}

// WARN: The code below (2 functions) is pure slop. No human oversight.
// TODO: review/refactor the code below.

func CreateInteractiveHostKeyCallback(knownHostsPath string) ssh.HostKeyCallback {
	khCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		log.Fatalf("failed to parse known_hosts: %v", err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := khCallback(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) == 0 {
				return handleUnknownHost(knownHostsPath, hostname, remote, key)
			}

			// Check if we have a key of the same type already
			for _, want := range keyErr.Want {
				if want.Key.Type() == key.Type() {
					fmt.Printf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n")
					fmt.Printf("@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!        @\n")
					fmt.Printf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n")
					fmt.Printf("IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!\n")
					fmt.Printf("Someone could be eavesdropping on you right now (man-in-the-middle attack)!\n")
					fmt.Printf("It is also possible that a host key has just been changed.\n")
					fmt.Printf("The fingerprint for the %s key sent by the remote host is\n%s.\n",
						key.Type(), ssh.FingerprintSHA256(key))
					fmt.Printf("Please contact your system administrator.\n")
					return fmt.Errorf("host key mismatch for %s: %w", hostname, err)
				}
			}

			// We know the host, but it's a new key type. Treat like unknown host.
			return handleUnknownHost(knownHostsPath, hostname, remote, key)
		}

		return err
	}
}

func handleUnknownHost(filePath, hostname string, remote net.Addr, key ssh.PublicKey) error {
	fingerprint := ssh.FingerprintSHA256(key)
	fmt.Printf("The authenticity of host '%s (%s)' can't be established.\n", hostname, remote.String())
	fmt.Printf("%s key fingerprint is %s.\n", key.Type(), fingerprint)
	fmt.Print("Are you sure you want to continue connecting (yes/no)? ")

	// Read standard input response
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "yes" && response != "y" {
		return errors.New("host key verification failed by user")
	}

	// 3. If yes, format the entry and append it to the known_hosts file
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open known_hosts for writing: %w", err)
	}
	defer f.Close()

	// Normalizes IP and hostname to match standard SSH file layout
	hostAddresses := []string{knownhosts.Normalize(hostname)}
	if remoteStr, _, err := net.SplitHostPort(remote.String()); err == nil && remoteStr != hostname {
		hostAddresses = append(hostAddresses, knownhosts.Normalize(remoteStr))
	}

	// Format line using the utility function from the library
	knownHostLine := knownhosts.Line(hostAddresses, key)
	if _, err := f.WriteString(knownHostLine + "\n"); err != nil {
		return fmt.Errorf("failed to write key to known_hosts: %w", err)
	}

	fmt.Printf("Warning: Permanently added '%s' (%s) to the list of known hosts.\n", hostname, key.Type())
	return nil
}

//  ---- End of slop ----
