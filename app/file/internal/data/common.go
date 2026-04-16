package data

import (
	pb "api/api/file/common/v1"
	userpb "api/api/user/common/v1"
	"common/constants"
	"common/hashid"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"entgo.io/ent/dialect/sql"
)

type (
	OrderDirection string
	PaginationArgs struct {
		UseCursorPagination bool
		Page                int
		PageToken           string
		PageSize            int
		OrderBy             string
		Order               OrderDirection
	}

	PageToken struct {
		Time          *time.Time `json:"time,omitempty"`
		ID            int        `json:"-"`
		IDHash        string     `json:"id,omitempty"`
		String        string     `json:"string,omitempty"`
		Int           int        `json:"int,omitempty"`
		StartWithFile bool       `json:"start_with_file,omitempty"`
	}

	UserIDCtx struct{}
)

const (
	OrderDirectionAsc  = OrderDirection("asc")
	OrderDirectionDesc = OrderDirection("desc")
)

var (
	ErrTooManyArguments = fmt.Errorf("too many arguments")
)

func pageTokenFromString(s string, hasher hashid.Encoder, idType int) (*PageToken, error) {
	sB64Decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 for page token: %w", err)
	}

	token := &PageToken{}
	if err := json.Unmarshal(sB64Decoded, token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal page token: %w", err)
	}

	id, err := hasher.Decode(token.IDHash, idType)
	if err != nil {
		return nil, fmt.Errorf("failed to decode id: %w", err)
	}

	if token.Time == nil {
		token.Time = &time.Time{}
	}

	token.ID = id
	return token, nil
}

func (p *PageToken) Encode(hasher hashid.Encoder, encodeFunc hashid.EncodeFunc) (string, error) {
	p.IDHash = encodeFunc(hasher, p.ID)
	res, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("failed to marshal page token: %w", err)
	}

	return base64.StdEncoding.EncodeToString(res), nil
}

// getOrderTerm returns the order term for ent.
func getOrderTerm(d OrderDirection) sql.OrderTermOption {
	switch d {
	case OrderDirectionDesc:
		return sql.OrderDesc()
	default:
		return sql.OrderAsc()
	}
}

func capPageSize(maxSQlParam, preferredSize, margin int) int {
	// Page size should not be bigger than max SQL parameter
	pageSize := preferredSize
	if maxSQlParam > 0 && pageSize > maxSQlParam-margin || pageSize == 0 {
		pageSize = maxSQlParam - margin
	}

	return pageSize
}

func UserInfoFromPbUser(user *userpb.User) *pb.UserInfo {
	if user == nil {
		return nil
	}
	ownerInfo := &pb.UserInfo{
		Id:       strconv.FormatInt(user.Id, 10),
		Nickname: user.Nick,
		Status:   GetUserStatus(user.Status),
	}
	if user.Avatar != nil {
		ownerInfo.Avatar = user.Avatar.Value
	}
	return ownerInfo
}

func GetUserStatus(status userpb.User_Status) string {
	switch status {
	case userpb.User_STATUS_ACTIVE:
		return constants.StatusActive
	case userpb.User_STATUS_INACTIVE:
		return constants.StatusInactive
	case userpb.User_STATUS_MANUAL_BANNED:
		return constants.StatusManualBanned
	default:
		return constants.StatusInactive
	}
}
