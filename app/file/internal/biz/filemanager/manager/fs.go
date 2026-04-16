package manager

import (
	pb "api/api/file/common/v1"
	"common/serializer"
	"context"
	"file/ent"
	"file/internal/biz/cluster"
	"file/internal/biz/filemanager/driver"
	"file/internal/biz/filemanager/driver/cos"
	"file/internal/biz/filemanager/driver/ks3"
	"file/internal/biz/filemanager/driver/local"
	"file/internal/biz/filemanager/driver/obs"
	"file/internal/biz/filemanager/driver/onedrive"
	"file/internal/biz/filemanager/driver/oss"
	"file/internal/biz/filemanager/driver/qiniu"
	"file/internal/biz/filemanager/driver/remote"
	"file/internal/biz/filemanager/driver/s3"
	"file/internal/biz/filemanager/driver/upyun"
	"file/internal/biz/filemanager/fs"
	"file/internal/data/types"
)

func (m *manager) LocalDriver(policy *ent.StoragePolicy) driver.Handler {
	if policy == nil {
		policy = &ent.StoragePolicy{Type: types.PolicyTypeLocal, Settings: &pb.PolicySetting{}}
	}
	return local.New(policy, m.l.Logger(), m.config)
}

func (m *manager) CastStoragePolicyOnSlave(ctx context.Context, policy *ent.StoragePolicy) *ent.StoragePolicy {
	if !m.stateless {
		return policy
	}

	nodeId := cluster.NodeIdFromContext(ctx)
	if policy.Type == types.PolicyTypeRemote {
		if nodeId != policy.NodeID {
			return policy
		}

		policyCopy := *policy
		policyCopy.Type = types.PolicyTypeLocal
		return &policyCopy
	} else if policy.Type == types.PolicyTypeLocal {
		policyCopy := *policy
		policyCopy.NodeID = nodeId
		policyCopy.Type = types.PolicyTypeRemote
		policyCopy.SetNode(&ent.Node{
			ID:       nodeId,
			Server:   cluster.MasterSiteUrlFromContext(ctx),
			SlaveKey: m.config.Slave.Secret,
		})
		return &policyCopy
	} else if policy.Type == types.PolicyTypeOss {
		policyCopy := *policy
		if policyCopy.Settings != nil {
			policyCopy.Settings.ServerSideEndpoint = ""
		}
	}

	return policy
}

func (m *manager) GetStorageDriver(ctx context.Context, policy *ent.StoragePolicy) (driver.Handler, error) {
	switch policy.Type {
	case types.PolicyTypeLocal:
		return local.New(policy, m.l.Logger(), m.config), nil
	case types.PolicyTypeRemote:
		return remote.New(ctx, policy, m.settings, m.config, m.l)
	case types.PolicyTypeOss:
		return oss.New(ctx, policy, m.settings, m.config, m.l.Logger(), m.mm.MimeDetector())
	case types.PolicyTypeCos:
		return cos.New(ctx, policy, m.settings, m.config, m.l.Logger(), m.mm.MimeDetector())
	case types.PolicyTypeS3:
		return s3.New(ctx, policy, m.settings, m.config, m.l.Logger(), m.mm.MimeDetector())
	case types.PolicyTypeKs3:
		return ks3.New(ctx, policy, m.settings, m.config, m.l.Logger(), m.mm.MimeDetector())
	case types.PolicyTypeObs:
		return obs.New(ctx, policy, m.settings, m.config, m.l.Logger(), m.mm.MimeDetector())
	case types.PolicyTypeQiniu:
		return qiniu.New(ctx, policy, m.settings, m.config, m.l.Logger(), m.mm.MimeDetector())
	case types.PolicyTypeUpyun:
		return upyun.New(ctx, policy, m.settings, m.config, m.l.Logger(), m.mm.MimeDetector())
	case types.PolicyTypeOd:
		return onedrive.New(ctx, policy, m.settings, m.config, m.l.Logger(), m.credm)
	default:
		return nil, ErrUnknownPolicyType
	}
}

func (m *manager) getEntityPolicyDriver(cxt context.Context, e fs.Entity, policyOverwrite *ent.StoragePolicy) (*ent.StoragePolicy, driver.Handler, error) {
	policyID := e.PolicyID()
	var (
		policy *ent.StoragePolicy
		err    error
	)
	if policyID == 0 {
		policy = &ent.StoragePolicy{Type: types.PolicyTypeLocal, Settings: &pb.PolicySetting{}}
	} else {
		if policyOverwrite != nil && policyOverwrite.ID == policyID {
			policy = policyOverwrite
		} else {
			policy, err = m.pc.GetPolicyByID(cxt, e.PolicyID())
			if err != nil {
				return nil, nil, serializer.NewError(serializer.CodeDBError, "failed to get policy", err)
			}
		}
	}

	d, err := m.GetStorageDriver(cxt, policy)
	if err != nil {
		return nil, nil, err
	}

	return policy, d, nil
}
