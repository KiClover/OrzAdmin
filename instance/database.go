package instance

import (
	"github.com/KiClover/orzfrp/conf"
	"github.com/KiClover/orzfrp/ent"
	"github.com/sirupsen/logrus"
)

func NewDatabase(conf *conf.Config) *ent.Client {
	db := ent.NewClient(
		// logger
		ent.Log(logrus.Error),
		ent.Driver(conf.DatabaseConf.NewNoCacheDriver()),
	)
	return db

}
