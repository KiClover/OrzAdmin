New-Item -ItemType Directory -Force -Path logs | Out-Null

function Start-Svc($name, $cmd) {

  Start-Process -FilePath "cmd.exe" -ArgumentList "/k", $cmd -WindowStyle Normal
  Write-Host "Started $name (new cmd window). Stop: close window or Ctrl+C."
}

Start-Svc "user"     "go run services/system/user/cmd/main.go -env=false -f=etc/user.yaml"
Start-Svc "setting"  "go run services/system/setting/cmd/main.go -env=false -f=etc/setting.yaml"
Start-Svc "msg"      "go run services/system/message/cmd/main.go -env=false -f=etc/msg.yaml"
Start-Svc "access" "go run services/system/access/cmd/main.go -env=false -f=etc/access.yaml"
Start-Svc "api-auth" "go run api/system/auth/cmd/main.go -env=false -f=etc/api/auth.yaml"
Start-Svc "api-user" "go run api/system/user/cmd/main.go -env=false -f=etc/api/user.yaml"
Start-Svc "api-permission" "go run api/system/permission/cmd/main.go -env=false -f=etc/api/permission.yaml"