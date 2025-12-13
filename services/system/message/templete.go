package message

import (
	ht "html/template"

	"github.com/sirupsen/logrus"
)

const (
	VerifyCodeTemplate = `<div>
  <includetail>
    <div align="center">
      <div class="open_email" style="margin-left: 8px; margin-top: 8px; margin-bottom: 8px; margin-right: 8px;">
        <div>
          <br>
          <span class="genEmailContent">
            <div id="cTMail-Wrap"
                             style="word-break: break-all;box-sizing:border-box;text-align:center;min-width:320px; max-width:660px; border:1px solid #f6f6f6; background-color:#f7f8fa; margin:auto; padding:20px 0 30px; font-family:'helvetica neue',PingFangSC-Light,arial,'hiragino sans gb','microsoft yahei ui','microsoft yahei',simsun,sans-serif">
              <div class="main-content" style="">
                <table style="width:100%;font-weight:300;margin-bottom:10px;border-collapse:collapse">
                  <tbody>
                    <tr style="font-weight:300">
                      <td style="width:3%;max-width:30px;">
                      </td>
                      <td style="max-width:600px;">
                        <p style="height:2px;background-color: #00a4ff;border: 0;font-size:0;padding:0;width:100%;margin-top:20px;">
                        </p>
                        <div id="cTMail-inner" style="background-color:#fff; padding:23px 0 20px;box-shadow: 0px 1px 1px 0px rgba(122, 55, 55, 0.2);text-align:left;">
                          <table style="width:100%;font-weight:300;margin-bottom:10px;border-collapse:collapse;text-align:left;">
                            <tbody>
                              <tr style="font-weight:300">
                                <td style="width:3.2%;max-width:30px;">
                                </td>
                                <td style="max-width:480px;text-align:left;">
                                  <h1 id="cTMail-title" style="font-size: 20px; line-height: 36px; margin: 0px 0px 22px;">
                                    注册验证码
                                  </h1>
                                  <p id="cTMail-userName" style="font-size:14px;color:#333; line-height:24px; margin:0;">
                                    
                                  </p>
                                  <p class="cTMail-content" style="line-height: 24px; margin: 6px 0px 0px; overflow-wrap: break-word; word-break: break-all;">
                                    <span style="color: rgb(51, 51, 51); font-size: 14px;">
                                      非常感谢您选择了我们的服务
                                    </span>
                                  </p>
                                  <p class="cTMail-content" style="line-height: 24px; margin: 6px 0px 0px; overflow-wrap: break-word; word-break: break-all;">
                                    <span style="color: rgb(51, 51, 51); font-size: 14px;">
                                      完成注册，请点击下面按钮验证邮箱。
                                      <span style="font-weight: bold;">
                                        非本人操作可忽略。
                                      </span>
                                    </span>
                                  </p>
                                  <p class="cTMail-content"
                                                               style="font-size: 14px; color: rgb(51, 51, 51); line-height: 24px; margin: 6px 0px 0px; word-wrap: break-word; word-break: break-all;">
                                    <a id="cTMail-btn"  title=""
                                                                   style="font-size: 16px; line-height: 45px; display: block; background-color: rgb(0, 164, 255); color: rgb(255, 255, 255); text-align: center; text-decoration: none; margin-top: 20px; border-radius: 3px;">
                                      {{.Code}}
                                    </a>
                                  </p>
                                  <p class="cTMail-content" style="line-height: 24px; margin: 6px 0px 0px; overflow-wrap: break-word; word-break: break-all;">
                                   

                                  <dl style="font-size: 14px; color: rgb(51, 51, 51); line-height: 18px;">
                                    <dd style="margin: 0px 0px 6px; padding: 0px; font-size: 12px; line-height: 22px;">
                                      <p id="cTMail-sender" style="font-size: 14px; line-height: 26px; word-wrap: break-word; word-break: break-all; margin-top: 32px;">
                                        此致
                                        <br>
                                        <strong>
                                          {{.SignName}}
                                        </strong>
                                      </p>
                                    </dd>
                                  </dl>
                                </td>
                                <td style="width:3.2%;max-width:30px;">
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>
                       
                        <div id="cTMail-copy" style="text-align:center; font-size:12px; line-height:18px; color:#999">
                          <table style="width:100%;font-weight:300;margin-bottom:10px;border-collapse:collapse">
                            <tbody>
                              <tr style="font-weight:300">
                                <td style="width:3.2%;max-width:30px;">
                                </td>
                                <td style="max-width:540px;">
                                  <p style="text-align:center; margin:20px auto 14px auto;font-size:12px;color:#999;">
                                    此为系统邮件，请勿回复。
                                                        
                                  <p id="cTMail-rights" style="max-width: 100%; margin:auto;font-size:12px;color:#999;text-align:center;line-height:22px;">
                                    <br>
                                    Copyright © 2025 {{.SignName}} All Rights Reserved.
                                  </p>
                                </td>
                                <td style="width:3.2%;max-width:30px;">
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>
                      </td>
                      <td style="width:3%;max-width:30px;">
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </span>
        </div>
      </div>
    </div>
  </includetail>
</div>`

	BasicBodyTemplate = `<div>
    <includetail>
        <div align="center">
            <div class="open_email" style="margin-left: 8px; margin-top: 8px; margin-bottom: 8px; margin-right: 8px;">
                <div>
                    <br>
                    <span class="genEmailContent">
            <div id="cTMail-Wrap"
                 style="word-break: break-all;box-sizing:border-box;text-align:center;min-width:320px; max-width:660px; border:1px solid #f6f6f6; background-color:#f7f8fa; margin:auto; padding:20px 0 30px; font-family:'helvetica neue',PingFangSC-Light,arial,'hiragino sans gb','microsoft yahei ui','microsoft yahei',simsun,sans-serif">
              <div class="main-content" style="">
                <table style="width:100%;font-weight:300;margin-bottom:10px;border-collapse:collapse">
                  <tbody>
                    <tr style="font-weight:300">
                      <td style="width:3%;max-width:30px;">
                      </td>
                      <td style="max-width:600px;">
                        <p style="height:2px;background-color: #00a4ff;border: 0;font-size:0;padding:0;width:100%;margin-top:20px;">
                        </p>
                        <div id="cTMail-inner" style="background-color:#fff; padding:23px 0 20px;box-shadow: 0px 1px 1px 0px rgba(122, 55, 55, 0.2);text-align:left;">
                          <table style="width:100%;font-weight:300;margin-bottom:10px;border-collapse:collapse;text-align:left;">
                            <tbody>
                              <tr style="font-weight:300">
                                <td style="width:3.2%;max-width:30px;">
                                </td>
                                <td style="max-width:480px;text-align:left;">
                                  <h1 id="cTMail-title" style="font-size: 20px; line-height: 36px; margin: 0px 0px 22px;">
                                    {{.Title}}
                                  </h1>
                                  <p id="cTMail-userName" style="font-size:14px;color:#333; line-height:24px; margin:0;">

                                  </p>
                                  <p class="cTMail-content" style="line-height: 24px; margin: 6px 0px 0px; overflow-wrap: break-word; word-break: break-all;">
                                    <span style="color: rgb(51, 51, 51); font-size: 14px;">
                                      {{.Content}}
                                    </span>
                                  </p>
                                  <p class="cTMail-content" style="line-height: 24px; margin: 6px 0px 0px; overflow-wrap: break-word; word-break: break-all;">


                                  <dl style="font-size: 14px; color: rgb(51, 51, 51); line-height: 18px;">
                                    <dd style="margin: 0px 0px 6px; padding: 0px; font-size: 12px; line-height: 22px;">
                                      <p id="cTMail-sender" style="font-size: 14px; line-height: 26px; word-wrap: break-word; word-break: break-all; margin-top: 32px;">
                                        此致
                                        <br>
                                        <strong>
                                          {{.SignName}}
                                        </strong>
                                      </p>
                                    </dd>
                                  </dl>
                                </td>
                                <td style="width:3.2%;max-width:30px;">
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>

                        <div id="cTMail-copy" style="text-align:center; font-size:12px; line-height:18px; color:#999">
                          <table style="width:100%;font-weight:300;margin-bottom:10px;border-collapse:collapse">
                            <tbody>
                              <tr style="font-weight:300">
                                <td style="width:3.2%;max-width:30px;">
                                </td>
                                <td style="max-width:540px;">
                                  <p style="text-align:center; margin:20px auto 14px auto;font-size:12px;color:#999;">
                                    此为系统邮件，请勿回复。

                                  <p id="cTMail-rights" style="max-width: 100%; margin:auto;font-size:12px;color:#999;text-align:center;line-height:22px;">
                                    <br>
                                    Copyright © 2025 {{.SignName}} All Rights Reserved.
                                  </p>
                                </td>
                                <td style="width:3.2%;max-width:30px;">
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>
                      </td>
                      <td style="width:3%;max-width:30px;">
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </span>
                </div>
            </div>
        </div>
    </includetail>
</div>`
)

func GetVerifyCodeTemplate() *ht.Template {
	htmlTpl, err := ht.New("htmltpl").Parse(VerifyCodeTemplate)
	if err != nil {
		logrus.Fatalf("failed to parse text template: %s", err)
	}
	return htmlTpl
}

func GetBasicBodyTemplate() *ht.Template {
	htmlTpl, err := ht.New("htmltpl").Parse(BasicBodyTemplate)
	if err != nil {
		logrus.Fatalf("failed to parse text template: %s", err)
	}
	return htmlTpl
}
