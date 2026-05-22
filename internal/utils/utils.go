package utils

import (
	"log"
	"strings"
	"strconv"
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
