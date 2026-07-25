// Copyright (c) 2026 MosaicPlane Authors
// SPDX-License-Identifier: Apache-2.0

package oidc

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/erda-project/erda-infra/base/servicehub"
	commonpb "github.com/erda-project/erda-proto-go/common/pb"
	"github.com/erda-project/erda-proto-go/core/user/pb"
	"github.com/erda-project/erda/internal/core/user/common"
	"github.com/erda-project/erda/pkg/common/apis"
)

const Source = "OIDC"

type Claims struct {
	Issuer   string
	Subject  string
	Username string
	Name     string
	Email    string
	Phone    string
	Avatar   string
}

type Interface interface {
	common.Interface
	UpsertIdentity(ctx context.Context, claims Claims) (*commonpb.UserInfo, error)
}

type Config struct {
	AdminSubjects string `file:"admin_subjects"`
}

type provider struct {
	pb.UnimplementedUserServiceServer
	Cfg *Config
	DB  *gorm.DB `autowired:"mysql-client"`
}

type userModel struct {
	ID          string    `gorm:"column:id;primary_key"`
	Issuer      string    `gorm:"column:issuer"`
	Subject     string    `gorm:"column:subject"`
	Username    string    `gorm:"column:username"`
	Nickname    string    `gorm:"column:nickname"`
	Email       string    `gorm:"column:email"`
	Phone       string    `gorm:"column:phone"`
	Avatar      string    `gorm:"column:avatar"`
	LastLoginAt time.Time `gorm:"column:last_login_at"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (userModel) TableName() string { return "mosaicplane_oidc_user" }

func (p *provider) Init(_ servicehub.Context) error {
	if p.DB == nil {
		return errors.New("OIDC user directory requires mysql-client")
	}
	return nil
}

func (p *provider) UpsertIdentity(_ context.Context, claims Claims) (*commonpb.UserInfo, error) {
	if strings.TrimSpace(claims.Issuer) == "" || strings.TrimSpace(claims.Subject) == "" {
		return nil, errors.New("OIDC issuer and subject are required")
	}

	now := time.Now().UTC()
	var model userModel
	err := p.DB.Where("issuer = ? AND subject = ?", claims.Issuer, claims.Subject).First(&model).Error
	if gorm.IsRecordNotFoundError(err) {
		model = userModel{
			ID: uuid.NewString(), Issuer: claims.Issuer, Subject: claims.Subject,
			CreatedAt: now,
		}
	} else if err != nil {
		return nil, errors.Wrap(err, "query OIDC user")
	}

	model.Username = firstNonEmpty(claims.Username, claims.Email, claims.Subject)
	model.Nickname = firstNonEmpty(claims.Name, model.Username)
	model.Email = claims.Email
	model.Phone = claims.Phone
	model.Avatar = claims.Avatar
	model.LastLoginAt = now
	model.UpdatedAt = now
	if err := p.DB.Save(&model).Error; err != nil {
		// A concurrent first login can win the unique issuer/subject insert.
		if err := p.DB.Where("issuer = ? AND subject = ?", claims.Issuer, claims.Subject).First(&model).Error; err != nil {
			return nil, errors.Wrap(err, "upsert OIDC user")
		}
	}
	if p.isAdminSubject(claims.Issuer, claims.Subject) {
		if err := p.ensureSystemAdmin(&model); err != nil {
			return nil, errors.Wrap(err, "grant OIDC system administrator")
		}
	}
	return model.userInfo(), nil
}

func (p *provider) isAdminSubject(issuer, subject string) bool {
	if p.Cfg == nil {
		return false
	}
	wanted := issuer + "|" + subject
	for _, entry := range strings.FieldsFunc(p.Cfg.AdminSubjects, func(r rune) bool { return r == ',' || r == '\n' || r == ';' }) {
		if strings.TrimSpace(entry) == wanted {
			return true
		}
	}
	return false
}

func (p *provider) ensureSystemAdmin(user *userModel) error {
	tx := p.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if recoverValue := recover(); recoverValue != nil {
			tx.Rollback()
			panic(recoverValue)
		}
	}()
	if err := tx.Exec(`INSERT INTO dice_member
		(scope_type, scope_id, scope_name, parent_id, org_id, project_id, project_name, application_id, application_name,
		 role, user_id, email, mobile, nick, avatar, user_sync_at, created_at, updated_at, name)
		VALUES ('sys', 0, '', 0, 0, 0, '', 0, '', 'Manager', ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE email=VALUES(email), mobile=VALUES(mobile), nick=VALUES(nick), avatar=VALUES(avatar),
		 user_sync_at=VALUES(user_sync_at), updated_at=VALUES(updated_at), name=VALUES(name)`,
		user.ID, user.Email, user.Phone, user.Nickname, user.Avatar, user.LastLoginAt, user.CreatedAt, user.UpdatedAt, user.Username).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Exec(`INSERT INTO dice_member_extra (created_at, updated_at, user_id, parent_id, scope_id, scope_type, resource_key, resource_value)
		SELECT ?, ?, ?, '0', '0', 'sys', 'role', 'Manager' FROM DUAL
		WHERE NOT EXISTS (SELECT 1 FROM dice_member_extra WHERE user_id=? AND scope_id='0' AND scope_type='sys' AND resource_key='role' AND resource_value='Manager')`,
		user.CreatedAt, user.UpdatedAt, user.ID, user.ID).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func (p *provider) GetUser(_ context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	user, err := p.getByID(req.UserID)
	if err != nil {
		return nil, err
	}
	return &pb.GetUserResponse{Data: user.userInfo()}, nil
}

func (p *provider) FindUsers(_ context.Context, req *pb.FindUsersRequest) (*pb.FindUsersResponse, error) {
	if len(req.IDs) == 0 {
		return &pb.FindUsersResponse{}, nil
	}
	var rows []userModel
	if err := p.DB.Where("id IN (?)", req.IDs).Find(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "find OIDC users")
	}
	byID := make(map[string]*commonpb.UserInfo, len(rows))
	for i := range rows {
		byID[rows[i].ID] = rows[i].userInfo()
	}
	result := make([]*commonpb.UserInfo, 0, len(rows))
	if req.KeepOrder {
		for _, id := range req.IDs {
			if id == common.SystemOperator {
				result = append(result, common.SystemUser)
			} else if user := byID[id]; user != nil {
				result = append(result, user)
			}
		}
	} else {
		for i := range rows {
			result = append(result, rows[i].userInfo())
		}
	}
	return &pb.FindUsersResponse{Data: result}, nil
}

func (p *provider) FindUsersByKey(_ context.Context, req *pb.FindUsersByKeyRequest) (*pb.FindUsersByKeyResponse, error) {
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return &pb.FindUsersByKeyResponse{}, nil
	}
	like := "%" + key + "%"
	var rows []userModel
	if err := p.DB.Where("username LIKE ? OR nickname LIKE ? OR email LIKE ? OR phone LIKE ?", like, like, like, like).
		Limit(100).Find(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "search OIDC users")
	}
	result := make([]*commonpb.UserInfo, 0, len(rows))
	for i := range rows {
		result = append(result, rows[i].userInfo())
	}
	return &pb.FindUsersByKeyResponse{Data: result}, nil
}

func (p *provider) UserPaging(_ context.Context, req *pb.UserPagingRequest) (*pb.UserPagingResponse, error) {
	db := p.DB.Model(&userModel{})
	for column, value := range map[string]string{"username": req.Name, "nickname": req.Nick, "email": req.Email, "phone": req.Phone} {
		if value = strings.TrimSpace(value); value != "" {
			db = db.Where(column+" LIKE ?", "%"+value+"%")
		}
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, errors.Wrap(err, "count OIDC users")
	}
	pageNo, pageSize := req.PageNo, req.PageSize
	if pageNo < 1 {
		pageNo = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var rows []userModel
	if err := db.Order("last_login_at DESC").Offset((pageNo - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "page OIDC users")
	}
	list := make([]*pb.ManagedUser, 0, len(rows))
	for i := range rows {
		list = append(list, rows[i].managedUser())
	}
	return &pb.UserPagingResponse{Total: total, List: list}, nil
}

func (p *provider) UserListLoginMethod(context.Context, *pb.UserListLoginMethodRequest) (*pb.UserListLoginMethodResponse, error) {
	return &pb.UserListLoginMethodResponse{Data: []*pb.UserLoginMethod{{DisplayName: Source}}}, nil
}

func (p *provider) UserMe(ctx context.Context, _ *pb.UserMeRequest) (*commonpb.UserInfo, error) {
	return p.me(ctx)
}

func (p *provider) Me(ctx context.Context, _ *pb.UserMeRequest) (*commonpb.UserInfo, error) {
	return p.me(ctx)
}

func (p *provider) UserEventWebhook(context.Context, *pb.UserEventWebhookRequest) (*pb.UserEventWebhookResponse, error) {
	return &pb.UserEventWebhookResponse{}, nil
}

func (p *provider) me(ctx context.Context) (*commonpb.UserInfo, error) {
	id := apis.GetUserID(ctx)
	if id == "" {
		return nil, errors.New("must provide user id")
	}
	row, err := p.getByID(id)
	if err != nil {
		return nil, err
	}
	return row.userInfo(), nil
}

func (p *provider) getByID(id string) (*userModel, error) {
	if id == common.SystemOperator {
		return nil, fmt.Errorf("system operator is not an OIDC user")
	}
	var row userModel
	if err := p.DB.Where("id = ?", id).First(&row).Error; err != nil {
		return nil, errors.Wrap(err, "get OIDC user")
	}
	return &row, nil
}

func (m *userModel) userInfo() *commonpb.UserInfo {
	return &commonpb.UserInfo{Id: m.ID, Name: m.Username, Nick: m.Nickname, Email: m.Email, Phone: m.Phone, Avatar: m.Avatar}
}

func (m *userModel) managedUser() *pb.ManagedUser {
	return &pb.ManagedUser{Id: m.ID, Name: m.Username, Nick: m.Nickname, Email: m.Email, Phone: m.Phone, Avatar: m.Avatar,
		LastLoginAt: timestamppb.New(m.LastLoginAt), Source: Source}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func init() {
	servicehub.Register("erda.core.user.oidc", &servicehub.Spec{
		Services:   []string{"erda.core.user.oidc"},
		Types:      []reflect.Type{reflect.TypeOf((*Interface)(nil)).Elem()},
		ConfigFunc: func() interface{} { return &Config{} },
		Creator:    func() servicehub.Provider { return &provider{} },
	})
}
