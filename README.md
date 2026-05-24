Coolify instance wide backups. Currently only supports creating a an encrypted local backup of the coolify's core plus all non database backups

Working on restoration but it should be super straightforward.

---

### Why? 

The current integrated backup solution in coolify only supports backing up coolify's db and other databases deployed on the instance. This is great until you need to clone an instance or move an instance between machines. 

This is the product of that need. And I need it yesterday!!!. Might upstream these changes into the official coolify cli when I get time. For now this is a simple tool to aid with my migration. 

### Usage

#### Backup
Create an encrypted local backup of a Coolify instance.
```bash
coolify-tools backup server.example.com ~/.ssh/id_ed25519
```

#### Restore
Restore a backup to a target machine.
```bash
coolify-tools restore target.example.com ~/.ssh/id_ed25519 .coolify/20260524_120000
```

#### Restore Database
Restore databases to running containers from a backup.
```bash
coolify-tools restoredb target.example.com ~/.ssh/id_ed25519 .coolify/20260524_120000
```

### Limitations

- [ ] Does not backup db images.
- [ ] Does not backup bind mounts. For now. 
- [ ] Only backs up running container's volumes
- [ ] No S3 support
- [ ] Does not tie a backup to a particular coolify version. Placeholder v4.0.0 used. 


### Feature plans

Features I plan to implement in the future. Purely based on my needs.

- centralize config: backupDir, archiveName format, defaultSSHPort, ..etc

- Clone command: self explanatory. backup + restore in one
- Multiple signers: currently only one ssh key is used to sign each tar archive. I think backups should be encrypted by a minimum of 2 keys: your personal key + a backup key. Or even more if in a large team for example
- Ability to cherrypick volumes: You might want to exclude some volumes from the backups. Rn you can do that by turning the container off but that's wasteful.
- Max size for volumes: I found myself transfering a 1 gig + volume of pure logs and I'd wanna skip that next time