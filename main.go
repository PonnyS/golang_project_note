package main

import (
	"fmt"
	"log"
	"net"
	"sync"

	"golang.org/x/sys/unix"
)

func transfer() *unix.SockaddrInet4 {
	var address [4]byte
	ip := net.ParseIP("127.0.0.1")
	copy(address[:], ip[12:16])
	sa := &unix.SockaddrInet4{
		Port: 12345,
		Addr: address,
	}
	return sa
}

func main() {
	var (
		fd  int
		err error
	)
	// socket
	if fd, err = unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0); err != nil {
		log.Fatal(err)
		return
	}
	// set option
	if err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		log.Fatal(err)
		return
	}
	if err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		log.Fatal(err)
		return
	}

	// bind
	sa := transfer()
	if err = unix.Bind(fd, sa); err != nil {
		log.Fatal("1 bind", err)
		return
	}

	if err = unix.Listen(fd, 255); err != nil {
		log.Fatal("1 listen", err)
		return
	}

	for {
		nfd, nsa, err := unix.Accept(fd)
		if err != nil {
			log.Fatal("1 accept", err)
			return
		}
		fmt.Printf("1来连接啦 %v %v\n", nfd, nsa)
	}

	return

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()

		if err = unix.Listen(fd, 255); err != nil {
			log.Fatal("1 listen", err)
			return
		}

		for {
			nfd, nsa, err := unix.Accept(fd)
			if err != nil {
				log.Fatal("1 accept", err)
				return
			}
			fmt.Printf("1来连接啦 %v %v\n", nfd, nsa)
		}
	}()

	go func() {
		if err = unix.Listen(fd, 255); err != nil {
			log.Fatal("2 listen", err)
			return
		}

		for {
			nfd, nsa, err := unix.Accept(fd)
			if err != nil {
				log.Fatal("2 accept", err)
				return
			}
			fmt.Printf("2来连接啦 %v %v\n", nfd, nsa)
		}
	}()

	wg.Wait()
}
