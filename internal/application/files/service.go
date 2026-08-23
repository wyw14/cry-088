package files

import (
	"context"
	"io"

	"github.com/wyw14/cry-088/internal/domain/audit"
	"github.com/wyw14/cry-088/internal/domain/organization"
	"github.com/wyw14/cry-088/internal/domain/shared"
	platform "github.com/wyw14/cry-088/internal/platform/files"
)

type Storage interface {
	Save(ctx context.Context, id, name, contentType string, source io.Reader) (platform.Metadata, error)
	Open(ctx context.Context, storageKey string) (io.ReadCloser, error)
}

type MetadataRepository interface {
	Insert(ctx context.Context, metadata platform.Metadata, organizationID, ownerID, projectID string) error
	Get(ctx context.Context, id string) (platform.Metadata, string, string, string, error)
}

type AuditRepository interface {
	Append(context.Context, audit.Event) error
}
type IDGenerator interface{ New(string) (string, error) }

type Service struct {
	Storage  Storage
	Metadata MetadataRepository
	Audits   AuditRepository
	IDs      IDGenerator
}

func (s Service) Upload(ctx context.Context, organizationID, projectID, name, contentType string, source io.Reader, actor organization.Principal) (platform.Metadata, error) {
	if !actor.CanAccessProject(organizationID, projectID) {
		return platform.Metadata{}, shared.New(shared.CodeForbidden, "file project is outside actor scope")
	}
	id, err := s.IDs.New("file")
	if err != nil {
		return platform.Metadata{}, err
	}
	metadata, err := s.Storage.Save(ctx, id, name, contentType, source)
	if err != nil {
		return platform.Metadata{}, err
	}
	if err := s.Metadata.Insert(ctx, metadata, organizationID, actor.EmployeeID, projectID); err != nil {
		return platform.Metadata{}, err
	}
	return metadata, nil
}

func (s Service) Download(ctx context.Context, id string, actor organization.Principal) (platform.Metadata, io.ReadCloser, error) {
	metadata, organizationID, _, projectID, err := s.Metadata.Get(ctx, id)
	if err != nil {
		return platform.Metadata{}, nil, err
	}
	if !actor.CanAccessProject(organizationID, projectID) {
		return platform.Metadata{}, nil, shared.New(shared.CodeForbidden, "file is outside actor scope")
	}
	reader, err := s.Storage.Open(ctx, metadata.StorageKey)
	if err != nil {
		return platform.Metadata{}, nil, err
	}
	return metadata, reader, nil
}
