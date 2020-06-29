package main

import (
	"fmt"
	"unsafe"
)

type user struct {
	name string
	age int
}

func main() {
	u := new(user)
	fmt.Println(*u)

	pName := (*string)(unsafe.Pointer(u))
	*pName = "张三"

	age := (*int)(unsafe.Pointer(uintptr(unsafe.Pointer(u)) + unsafe.Offsetof(u.age)))
	*age = 20

	fmt.Println(*u)

	s := "张三"
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	buf := *(*[]byte)(unsafe.Pointer(&h))
	s1 := []byte(s)
	fmt.Printf("%+v\n%+v", buf, s1)
}
