package user

import (
	"context"

	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	userEnt "github.com/KiClover/orzAdmin/ent/user"
	"github.com/KiClover/orzAdmin/pkg/utils"
)

// AdminName 是默认管理员用户名。默认管理员只在第一次初始化（is_first=true）
// 时补建；之后的初始化不会按名字重建账号，改名或改密码都不会被重置。
const AdminName = "admin"

// AdminPassword 是默认管理员密码占位符，构建期会被替换成真实值。
const AdminPassword = "password"

// BootstrapAdminSeed 描述默认管理员的初始资料。
type BootstrapAdminSeed struct {
	Name        string
	Password    string
	Description string
	Mobile      string
	Email       string
	Status      uint
	Gender      uint
}

// BootstrapAdminUserSeed 返回默认管理员账号的初始资料。
func BootstrapAdminUserSeed() BootstrapAdminSeed {
	return BootstrapAdminSeed{
		Name:        AdminName,
		Password:    AdminPassword,
		Description: "administrator",
		Mobile:      "10000000000",
		Email:       "kiclover@email.cn",
		Status:      0,
		Gender:      0,
	}
}

// ensureBootstrapAdmin 在第一次初始化时补建默认管理员。
//
// 幂等：同名账号已存在时直接返回，不覆盖密码、资料或状态；并发首次初始化
// 由唯一索引兜底，命中约束冲突同样视为已完成。
func ensureBootstrapAdmin(ctx context.Context, database *db.UserServiceDB) error {
	exists, err := database.User.Query().Where(userEnt.Name(AdminName)).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	seed := BootstrapAdminUserSeed()
	// Crypt password
	encrypted, err := utils.HashPassword(seed.Password)
	if err != nil {
		return err
	}
	_, err = database.User.Create().
		SetName(seed.Name).
		SetPwd(encrypted).
		SetDescription(seed.Description).
		SetMobile(seed.Mobile).
		SetEmail(seed.Email).
		SetStatus(seed.Status).
		SetGender(seed.Gender).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			// 并发初始化时另一个请求已经建好，视为完成。
			return nil
		}
		return err
	}
	return nil
}
