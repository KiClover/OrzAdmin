package kerr

import "github.com/sirupsen/logrus"

func EntDefaultErr(err error) {
	logrus.Errorf("ent default err:%v", err)
}
