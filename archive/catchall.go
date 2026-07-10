package archive

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/Heracraft/coolify-tools/internal/utils"
)



func createTarArchive(client *ssh.Client) string {
	session, err := client.NewSession()

	if err != nil {
		log.Fatalf("failed to create ssh session %v", err)
	}

	out, err := session.CombinedOutput("du  /data/coolify -d 0 | awk '{print $1}'")

	utils.HandleErr("failed to check dir size", err, "%s", string(out))

	coolifyDirSize := utils.ParseOutputAsNumber(out)

	out, err = session.CombinedOutput("df / --output=avail | tail -1")

	utils.HandleErr("failed to check avail disk space", err, "%s", string(out))

	availableSpace := utils.ParseOutputAsNumber(out)

	if coolifyDirSize > availableSpace {
		log.Fatalf("Not enough space left on device to create backup")
		// TODO: offer to download everything to the client? or push to s3??
	}
	timestamp := time.Now().Format("20060102_150405")

	backupPath := fmt.Sprintf("/opt/backups/coolify-backup-%s.tar.gz", timestamp)

	cmd := fmt.Sprintf("mkdir -p /opt/backups && tar -czf %s /data/coolify", backupPath)

	out, err = session.CombinedOutput(cmd)

	utils.HandleErr("failed to create archive", err, "%s", string(out))

	return string(out)

}
