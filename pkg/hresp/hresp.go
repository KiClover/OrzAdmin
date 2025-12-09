package hresp

import (
	"fmt"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/sirupsen/logrus"
)

func BaseSuccess(c *app.RequestContext) {
	c.JSON(200, map[string]interface{}{
		"code": 0,
		"msg":  "请求成功",
	})
}

func BaseSuccessMsg(c *app.RequestContext, Msg string) {
	c.JSON(200, map[string]interface{}{
		"code": 0,
		"msg":  Msg,
	})
}

func BaseSuccessMsgWithBody(c *app.RequestContext, Msg string, body interface{}) {
	c.JSON(200, map[string]interface{}{
		"code": 0,
		"msg":  Msg,
		"data": body,
	})
}

func BaseSuccessWithBody(c *app.RequestContext, body interface{}) {
	c.JSON(200, map[string]interface{}{
		"code": 0,
		"msg":  "请求成功",
		"data": body,
	})
}

func BaseErrMsg(c *app.RequestContext, code int32, msg string, err error) {
	c.JSON(200, map[string]interface{}{
		"code": code,
		"msg":  msg,
	})
}

func KitexCallErr(c *app.RequestContext, serviceName string, err error) {
	logrus.Errorf("service %s call err:%v", serviceName, err)
	c.JSON(200, map[string]interface{}{
		"code": 1,
		"msg":  fmt.Sprintf("%s 服务调用异常", serviceName),
	})
}

func ReqBindErr(c *app.RequestContext, err error) {
	logrus.Errorf("request bind err:%v", err)
	c.JSON(200, map[string]interface{}{
		"code": 2,
		"msg":  "请求参数异常",
	})
}
