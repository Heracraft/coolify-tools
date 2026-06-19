package docker

import "strings"

type DBEngine string

const (
	EnginePostgres   DBEngine = "postgres"
	EngineMysql      DBEngine = "mysql"
	EngineMariadb    DBEngine = "mariadb"
	EngineMongo      DBEngine = "mongo"
	EngineRedis      DBEngine = "redis"
	EngineClickhouse DBEngine = "clickhouse"
)

var DBKeywords = []DBEngine{
	EnginePostgres, EngineMysql, EngineMariadb, EngineMongo, EngineRedis, EngineClickhouse,
}

func IsDatabaseImage(image string) bool {
	image = strings.ToLower(image)

	for _, kw := range DBKeywords {
		if strings.Contains(image, string(kw)) {
			return true
		}
	}
	return false
}

func CategorizeVolumes(containers []Container) (fileVolumes []Container, dbVolumes []Container) {
	fileVSet := make(map[string]Container)
	dbVSet := make(map[string]Container)

	for _, c := range containers {
		if IsDatabaseImage(c.Config.Image) {
			dbVSet[c.Name] = c
		} else {
			fileVSet[c.Name] = c
		}
	}

	for containerName := range fileVSet {
		fileVolumes = append(fileVolumes, fileVSet[containerName])
	}

	for containerName := range dbVSet {
		dbVolumes = append(dbVolumes, dbVSet[containerName])
	}

	return fileVolumes, dbVolumes
}

