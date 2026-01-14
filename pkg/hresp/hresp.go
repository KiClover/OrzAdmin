package hresp

import (
	"fmt"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/sirupsen/logrus"
)

func BaseSuccess(c *app.RequestContext) {
	c.JSON(200, &Resp{
		Code: 0,
		Msg:  "请求成功",
	})
}

func BaseSuccessMsg(c *app.RequestContext, Msg string) {
	c.JSON(200, &Resp{
		Code: 0,
		Msg:  Msg,
	})
}

func BaseSuccessMsgWithBody(c *app.RequestContext, Msg string, body interface{}) {
	c.JSON(200, &RespData[interface{}]{
		Code: 0,
		Msg:  Msg,
		Data: body,
	})
}

func BaseSuccessWithBody(c *app.RequestContext, body interface{}) {
	c.JSON(200, &RespData[interface{}]{
		Code: 0,
		Msg:  "请求成功",
		Data: body,
	})
}

func BaseErrMsg(c *app.RequestContext, code int32, msg string, err error) {
	if err != nil {
		logrus.Errorf("request err:%v", err)
	}
	c.JSON(200, &Resp{
		Code: code,
		Msg:  msg,
	})
}

func NotSuccessErr(c *app.RequestContext, code int, bodyCode int32, msg string) {
	c.JSON(code, &Resp{
		Code: bodyCode,
		Msg:  msg,
	})
}

func KitexCallErr(c *app.RequestContext, serviceName string, err error) {
	logrus.Errorf("service %s call err:%v", serviceName, err)
	c.JSON(200, &Resp{
		Code: 1,
		Msg:  fmt.Sprintf("%s 服务调用异常,异常: %v", serviceName, err),
	})
}

func ReqBindErr(c *app.RequestContext, err error) {
	logrus.Errorf("request bind err:%v", err)
	c.JSON(200, &Resp{
		Code: 2,
		Msg:  "请求参数异常",
	})
}
