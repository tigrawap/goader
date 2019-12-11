// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package main

import (
	"syscall"
)

func mknod(filename string) error {
	return syscall.Mknod(filename, 0666, 0)
}
