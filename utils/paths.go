package utils

import (
	"log"
	"os"
	"path"
)

var workingDirectory string
func init() {
	wd, err := os.Getwd()
	if err != nil {
		log.Panic(err)
	}
	workingDirectory = wd
}

func GetAbsolute(p string) string{
	if path.IsAbs(p){
		return p
	}else{
		return path.Join(workingDirectory, p)
	}
}
