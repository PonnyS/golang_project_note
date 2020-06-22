package main

import (
	//"github.com/panjf2000/ants/v2"
	"golang_project_note/ants"
	"time"
)

func main() {
	pool, _ := ants.NewPoolWithFunc(10, func(i interface{}) {
		time.Sleep(time.Duration(i.(int)) * time.Millisecond)
		println(i.(int))
	})
	defer pool.Release()

	_ = pool.Invoke(10)
	_ = pool.Invoke(20)
	_ = pool.Invoke(30)

	time.Sleep(1*time.Second)
}
