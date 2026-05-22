package ssh

import (
	"errors"
	"fmt"
	"log"
	"syscall"

	"net"
	"os"

	"golang.org/x/crypto/ssh"
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
			var passphrase []byte;

			if flagPassphrase == "" {

				fmt.Print("Enter key passphrase: ")
				passpharse, pErr := term.ReadPassword(int(syscall.Stdin))

				utils.HandleErr("failed to read password", pErr)

				defer func() {
					for i := range passpharse {
						passpharse[i] = 0
					}
				}()
			} else{
				passphrase := []byte(flagPassphrase)

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

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(
				signer,
			),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		//TODO: implement known hosts stuff
		// HostKeyCallback:ssh.ParseKnownHosts(),
	}

	remoteAddr := net.JoinHostPort(hostname, sshPort)

	client, err := ssh.Dial("tcp", remoteAddr, config)

	if err != nil {
		log.Fatalf("Failed to establish ssh connection: %v", err)
	}

	return client, signer
}
