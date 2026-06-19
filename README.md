Coolify instance wide backups. Currently only supports creating a an encrypted local backup of the coolify core, all file volumes and specific db engines. 

---

### Why? 

The current integrated backup solution in coolify only supports backing up coolify's db and other databases deployed on the instance. This is great until you need to clone an instance or move an instance between machines. 

This is the product of that need. And I need it yesterday!!!. Might upstream these changes into the official coolify cli when I get time. For now this is a simple tool to aid with my migration. 

### Usage

### Global Flags

These flags can be used with any command:

- `-u, --username <string>`: SSH Username (default "root")
- `-p, --port <string>`: Custom SSH port (default "22")
- `--passphrase <string>`: Private key passphrase. Extremely unsafe, logged in history, process info and etc. Use at your own risk

#### Backup
Create an encrypted local backup of a Coolify instance (or specific containers).
```bash
coolify-tools backup server.example.com id_ed25519 [target-container] [flags]
```

**Flags:**

- `-o, --out <string>`: Where to save the archives (default ".coolify")
- `--clean`: Include clean-up instructions (like DROP TABLE) in database dumps

#### Restore
Restore a backup to a target machine.
```bash
coolify-tools restore target.example.com id_ed25519 .coolify/20260524_120000 [target-container] [flags]
```

**Flags:**

- `--clean`: Wipe existing data before restoration

### Targeting Specific Containers

Both `backup` and `restore` commands support an optional `[target-container]` argument to target specific container(s) and their associated volumes.

#### For `backup`:
The `[target-container]` can match containers by:
- **Container Name**: Matches by exact name or substring (e.g., `actual` matches `ck10ww0sxs8s21wwkks4s8wok_actual-data`).
- **Container ID**: Matches by exact ID or prefix (e.g., `a1b2c3d4`).
- **Docker Compose Project**: Matches all containers with the `com.docker.compose.project` label equal to the target string.
- **Docker Compose Service**: Matches all containers with the `com.docker.compose.service` label equal to the target string.

#### For `restore`:
Matches using backup metadata:
- **Container Name**: The volume's stored container name is equal to or prefixed by the target string.
- **Volume Name**: The volume's stored name is equal to or prefixed by the target string.

#### Restore Database
Restore databases to running containers from a backup.
```bash
coolify-tools restoredb target.example.com ~/.ssh/id_ed25519 .coolify/20260524_120000 [flags]
```

or a specific container

```bash
coolify-tools restoredb target.example.com ~/.ssh/id_ed25519 .coolify/20260524_120000 [flags] <container-name>
```

#### Clone
Clone a Coolify instance (or specific container) from a source host directly to a target host in one step.
```bash
coolify-tools clone source.example.com target.example.com id_ed25519 [target-container] [flags]
```

**Flags:**

- `--keep`: Retain the local backup archive after the clone completes (default: `true`)
- `--clean`: Wipe existing data on target before restoration

### Limitations

- [ ] Does not backup bind mounts. For now. 
- [ ] Only backs up running container's volumes
- [ ] No S3 support
- [ ] Does not tie a backup to a particular coolify version. Placeholder v4.0.1 used. 


### Feature plans

Features I plan to implement in the future. Purely based on my needs.

- centralize config: backupDir, archiveName format, defaultSSHPort, ..etc

- [x] Clone command: self explanatory. backup + restore in one
- Multiple signers: currently only one ssh key is used to sign each tar archive. I think backups should be encrypted by a minimum of 2 keys: your personal key + a backup key. Or even more if in a large team for example
- Ability to cherrypick volumes: You might want to exclude some volumes from the backups. Rn you can do that by turning the container off but that's wasteful. I found myself transfering a 1GB+ volume of pure logs and I'd wanna skip that next time
- Max size for volumes