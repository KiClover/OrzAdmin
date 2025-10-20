package main

import (
	user "github.com/KiClover/orzfrp/kitex_gen/system/user/systemuserservice"
	user2 "github.com/KiClover/orzfrp/service/system/user"
	"log"
)

func main() {
	svr := user.NewServer(new(user2.SystemUserServiceImpl))

	err := svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
