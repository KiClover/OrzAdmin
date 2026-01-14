package hresp

type Resp struct {
	Code int32  `json:"code"`
	Msg  string `json:"msg"`
}

type RespData[T any] struct {
	Code int32  `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}
