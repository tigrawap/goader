// windows

package main

import (
	"os"
)

func mknod(filename string) error {
	fd, err := os.OpenFile(filename, os.O_RDONLY|os.O_CREATE, 0666)
	if err == nil {
		fd.Close()
	}
	return err
}
