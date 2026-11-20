package user

import (
	"context"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/KiClover/orzAdmin/conf"
	"github.com/KiClover/orzAdmin/db"
	"github.com/KiClover/orzAdmin/ent"
	"github.com/KiClover/orzAdmin/ent/migrate"
	"github.com/KiClover/orzAdmin/ent/predicate"
	userEnt "github.com/KiClover/orzAdmin/ent/user"
	internallog "github.com/KiClover/orzAdmin/internal/log"
	base "github.com/KiClover/orzAdmin/kitex_gen/base"
	user "github.com/KiClover/orzAdmin/kitex_gen/system/user"
	"github.com/KiClover/orzAdmin/pkg/kerr"
	"github.com/KiClover/orzAdmin/pkg/utils"
	"go.uber.org/fx"
)

const ServiceName = "system-user"

// logger 让 user 服务的日志统一带 [USER] 前缀。
var logger = internallog.WithModule(internallog.ModuleUser)

// SystemUserService implements the last service interface defined in the IDL.
type SystemUserService struct {
	fx.In
	DB    *db.UserServiceDB
	Cfg   *conf.Config
	DBAll *ent.Client
}

var _ user.SystemUserService = (*SystemUserService)(nil)

func NewSystemUserService(svc SystemUserService) user.SystemUserService {
	return &svc
}

func (s *SystemUserService) InitDatabase(ctx context.Context, req *user.InitDatabaseReq) (res *base.BaseResp, err error) {
	// 表结构由 user 服务用自己的 Ent 实例同步，失败时进程内直接报错。
	tables := []*schema.Table{migrate.UsersTable}
	if err := migrate.Create(ctx, s.DBAll.Schema, tables, schema.WithForeignKeys(false)); err != nil {
		logger.Errorf("init database err: %v", err)
		return nil, err
	}
	// is_first 只在第一次初始化时补建默认管理员；后续调用跳过，
	// 避免管理员改名或被删除后被重新创建。
	if req.GetIsFirst() {
		if err := ensureBootstrapAdmin(ctx, s.DB); err != nil {
			logger.Errorf("init bootstrap admin err: %v", err)
			return nil, err
		}
	}
	return &base.BaseResp{Msg: "success"}, nil
}

func (s *SystemUserService) GetBootstrapAdmin(ctx context.Context, req *base.Empty) (res *user.BootstrapAdmin, err error) {
	// 首次初始化后由初始化 API 取回管理员 ID，再交给 access 完成首次授权；
	// 这里只按固定用户名读取，不创建也不修改任何账号。
	u, err := s.DB.User.Query().Where(userEnt.Name(AdminName)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			exists := false
			return &user.BootstrapAdmin{Exists: &exists}, nil
		}
		kerr.EntDefaultErr(err)
		return nil, err
	}
	exists := true
	id := int64(u.ID)
	return &user.BootstrapAdmin{Exists: &exists, Id: &id}, nil
}

func (s *SystemUserService) CreateUser(ctx context.Context, req *user.UserInfo) (resp *base.BaseIDInt64Resp, err error) {
	// Crypt password
	pwd, err := utils.HashPassword(*req.Password)
	if err != nil {
		logger.Errorf("crypt password err: %v", err)
		return nil, err
	}
	u := s.DB.User.Create().
		SetName(*req.Name).
		SetPwd(pwd)
	if req.Status != nil {
		u = u.SetStatus(uint(*req.Status))
	}
	if req.Email != nil {
		u = u.SetEmail(utils.NormalizeEmail(*req.Email))
	}
	if req.Gender != nil {
		u = u.SetGender(uint(*req.Gender))
	}
	if req.Mobile != nil {
		u = u.SetMobile(*req.Mobile)
	}
	if req.Avatar != nil {
		u = u.SetAvatar(*req.Avatar)
	}
	r, err := u.Save(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &base.BaseIDInt64Resp{
		Id:  int64(r.ID),
		Msg: "success",
	}, nil
}

func (s *SystemUserService) UpdateUser(ctx context.Context, req *user.UserInfo) (resp *base.BaseResp, err error) {
	u := s.DB.User.UpdateOneID(uint64(*req.Id))
	var pwd string
	if req.Password != nil {
		pwd, err = utils.HashPassword(*req.Password)
		if err != nil {
			logger.Errorf("crypt password err: %v", err)
			return nil, err
		}
		u = u.SetPwd(pwd)
	}
	if req.Name != nil {
		u = u.SetName(*req.Name)
	}
	if req.Status != nil {
		u = u.SetStatus(uint(*req.Status))
	}
	if req.Email != nil {
		u = u.SetEmail(utils.NormalizeEmail(*req.Email))
	}
	if req.Gender != nil {
		u = u.SetGender(uint(*req.Gender))
	}
	if req.Mobile != nil {
		u = u.SetMobile(*req.Mobile)
	}
	if req.Avatar != nil {
		u = u.SetAvatar(*req.Avatar)
	}
	if req.Description != nil {
		u = u.SetDescription(*req.Description)
	}
	err = u.Exec(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s *SystemUserService) DeleteUser(ctx context.Context, req *base.IDInt64Req) (res *base.BaseResp, err error) {
	err = s.DB.User.DeleteOneID(uint64(req.Id)).Exec(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &base.BaseResp{
		Msg: "success",
	}, nil
}

func (s *SystemUserService) GetUserList(ctx context.Context, req *user.GetUserListReq) (res *user.GetUserListResp, err error) {
	var predicates []predicate.User
	if req.Mobile != nil {
		predicates = append(predicates, userEnt.MobileEQ(*req.Mobile))
	}

	if req.Name != nil {
		predicates = append(predicates, userEnt.NameContains(*req.Name))
	}

	if req.Email != nil {
		predicates = append(predicates, userEnt.EmailEQ(utils.NormalizeEmail(*req.Email)))
	}

	if req.Status != nil {
		predicates = append(predicates, userEnt.StatusEQ(uint(*req.Status)))
	}
	r, err := s.DB.User.Query().Where(predicates...).Offset(int((req.Page - 1) * req.PageSize)).Limit(int(req.PageSize)).All(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	total, err := s.DB.User.Query().Where(predicates...).Count(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	var list []*user.UserInfo
	for _, v := range r {
		list = append(list, &user.UserInfo{
			Id:       utils.GetPointer(int64(v.ID)),
			Name:     utils.GetPointer(v.Name),
			Gender:   utils.GetPointer(uint32(v.Gender)),
			Mobile:   utils.GetPointer(v.Mobile),
			Email:    utils.GetPointer(v.Email),
			Avatar:   utils.GetPointer(v.Avatar),
			Password: utils.GetPointer(v.Pwd),
			Status:   utils.GetPointer(uint32(v.Status)),
		})
	}
	return &user.GetUserListResp{
		Total: int64(total),
		Data:  list,
	}, nil
}

func (s *SystemUserService) GetUserByEmail(ctx context.Context, req *user.GetUserByEmailReq) (resp *user.UserInfo, err error) {
	r, err := s.DB.User.Query().Where(userEnt.Email(utils.NormalizeEmail(*req.Email))).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &user.UserInfo{
		Id:       utils.GetPointer(int64(r.ID)),
		Name:     utils.GetPointer(r.Name),
		Gender:   utils.GetPointer(uint32(r.Gender)),
		Mobile:   utils.GetPointer(r.Mobile),
		Email:    utils.GetPointer(r.Email),
		Avatar:   utils.GetPointer(r.Avatar),
		Password: utils.GetPointer(r.Pwd),
		Status:   utils.GetPointer(uint32(r.Status)),
	}, nil
}

func (s *SystemUserService) GetUserByMobile(ctx context.Context, req *user.GetUserByMobileReq) (resp *user.UserInfo, err error) {
	r, err := s.DB.User.Query().Where(userEnt.Mobile(*req.Mobile)).Only(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &user.UserInfo{
		Id:       utils.GetPointer(int64(r.ID)),
		Name:     utils.GetPointer(r.Name),
		Gender:   utils.GetPointer(uint32(r.Gender)),
		Mobile:   utils.GetPointer(r.Mobile),
		Email:    utils.GetPointer(r.Email),
		Avatar:   utils.GetPointer(r.Avatar),
		Password: utils.GetPointer(r.Pwd),
		Status:   utils.GetPointer(uint32(r.Status)),
	}, nil
}

func (s *SystemUserService) GetUserById(ctx context.Context, req *user.GetUserByIdReq) (res *user.UserInfo, err error) {
	r, err := s.DB.User.Get(ctx, uint64(*req.Id))
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &user.UserInfo{
		Id:       utils.GetPointer(int64(r.ID)),
		Name:     utils.GetPointer(r.Name),
		Gender:   utils.GetPointer(uint32(r.Gender)),
		Mobile:   utils.GetPointer(r.Mobile),
		Email:    utils.GetPointer(r.Email),
		Avatar:   utils.GetPointer(r.Avatar),
		Password: utils.GetPointer(r.Pwd),
		Status:   utils.GetPointer(uint32(r.Status)),
	}, nil
}

func (s *SystemUserService) GetUserIsExist(ctx context.Context, req *user.GetUserIsExistReq) (resp *user.GetUserIsExistResp, err error) {
	q := s.DB.User.Query()
	if req.Email != nil {
		q = q.Where(userEnt.Email(utils.NormalizeEmail(*req.Email)))
	}
	if req.Mobile != nil {
		q = q.Where(userEnt.Mobile(*req.Mobile))
	}
	if req.Id != nil {
		q = q.Where(userEnt.ID(uint64(*req.Id)))
	}
	exist, err := q.Exist(ctx)
	if err != nil {
		kerr.EntDefaultErr(err)
		return nil, err
	}
	return &user.GetUserIsExistResp{
		IsExist: exist,
	}, nil
}
