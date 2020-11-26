package godo

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	registryPath = "/v2/registry"
	// RegistryServer is the hostname of the DigitalOcean registry service
	RegistryServer = "registry.digitalocean.com"
)

// RegistryService is an interface for interfacing with the Registry endpoints
// of the DigitalOcean API.
// See: https://developers.digitalocean.com/documentation/v2#registry
type RegistryService interface {
	Create(context.Context, *RegistryCreateRequest) (*Registry, *Response, error)
	Get(context.Context) (*Registry, *Response, error)
	Delete(context.Context) (*Response, error)
	DockerCredentials(context.Context, *RegistryDockerCredentialsRequest) (*DockerCredentials, *Response, error)
	ListRepositories(context.Context, string, *ListOptions) ([]*Repository, *Response, error)
	ListRepositoryTags(context.Context, string, string, *ListOptions) ([]*RepositoryTag, *Response, error)
	DeleteTag(context.Context, string, string, string) (*Response, error)
	DeleteManifest(context.Context, string, string, string) (*Response, error)
	StartGarbageCollection(context.Context, string) (*GarbageCollection, *Response, error)
	GetGarbageCollection(context.Context, string) (*GarbageCollection, *Response, error)
	ListGarbageCollections(context.Context, string, *ListOptions) ([]*GarbageCollection, *Response, error)
	UpdateGarbageCollection(context.Context, string, string, *UpdateGarbageCollectionRequest) (*GarbageCollection, *Response, error)
	GetOptions(context.Context) (*RegistryOptions, *Response, error)
	GetSubscription(context.Context) (*RegistrySubscription, *Response, error)
	UpdateSubscription(context.Context, *RegistrySubscriptionUpdateRequest) (*RegistrySubscription, *Response, error)
}

var _ RegistryService = &RegistryServiceOp{}

// RegistryServiceOp handles communication with Registry methods of the DigitalOcean API.
type RegistryServiceOp struct {
	client *Client
}

// RegistryCreateRequest represents a request to create a registry.
type RegistryCreateRequest struct {
	Name                 string `json:"name,omitempty"`
	SubscriptionTierSlug string `json:"subscription_tier_slug,omitempty"`
}

// RegistryDockerCredentialsRequest represents a request to retrieve docker
// credentials for a registry.
type RegistryDockerCredentialsRequest struct {
	ReadWrite     bool `json:"read_write"`
	ExpirySeconds *int `json:"expiry_seconds,omitempty"`
}

// Registry represents a registry.
type Registry struct {
	Name      string    `json:"name,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Repository represents a repository
type Repository struct {
	RegistryName string         `json:"registry_name,omitempty"`
	Name         string         `json:"name,omitempty"`
	LatestTag    *RepositoryTag `json:"latest_tag,omitempty"`
	TagCount     uint64         `json:"tag_count,omitempty"`
}

// RepositoryTag represents a repository tag
type RepositoryTag struct {
	RegistryName        string    `json:"registry_name,omitempty"`
	Repository          string    `json:"repository,omitempty"`
	Tag                 string    `json:"tag,omitempty"`
	ManifestDigest      string    `json:"manifest_digest,omitempty"`
	CompressedSizeBytes uint64    `json:"compressed_size_bytes,omitempty"`
	SizeBytes           uint64    `json:"size_bytes,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

type registryRoot struct {
	Registry *Registry `json:"registry,omitempty"`
}

type repositoriesRoot struct {
	Repositories []*Repository `json:"repositories,omitempty"`
	Links        *Links        `json:"links,omitempty"`
	Meta         *Meta         `json:"meta"`
}

type repositoryTagsRoot struct {
	Tags  []*RepositoryTag `json:"tags,omitempty"`
	Links *Links           `json:"links,omitempty"`
	Meta  *Meta            `json:"meta"`
}

// GarbageCollection represents a garbage collection.
type GarbageCollection struct {
	UUID         string    `json:"uuid"`
	RegistryName string    `json:"registry_name"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	BlobsDeleted uint64    `json:"blobs_deleted"`
	FreedBytes   uint64    `json:"freed_bytes"`
}

type garbageCollectionRoot struct {
	GarbageCollection *GarbageCollection `json:"garbage_collection,omitempty"`
}

type garbageCollectionsRoot struct {
	GarbageCollections []*GarbageCollection `json:"garbage_collections,omitempty"`
	Links              *Links               `json:"links,omitempty"`
	Meta               *Meta                `json:"meta"`
}

// UpdateGarbageCollectionRequest represents a request to update a garbage
// collection.
type UpdateGarbageCollectionRequest struct {
	Cancel bool `json:"cancel"`
}

// RegistryOptions are options for users when creating or updating a registry.
type RegistryOptions struct {
	SubscriptionTiers []*RegistrySubscriptionTier `json:"subscription_tiers,omitempty"`
}

type registryOptionsRoot struct {
	Options *RegistryOptions `json:"options"`
}

// RegistrySubscriptionTier is a subscription tier for container registry.
type RegistrySubscriptionTier struct {
	Name                   string `json:"name"`
	Slug                   string `json:"slug"`
	IncludedRepositories   uint64 `json:"included_repositories"`
	IncludedStorageBytes   uint64 `json:"included_storage_bytes"`
	AllowStorageOverage    bool   `json:"allow_storage_overage"`
	IncludedBandwidthBytes uint64 `json:"included_bandwidth_bytes"`
	MonthlyPriceInCents    uint64 `json:"monthly_price_in_cents"`
	Eligible               bool   `json:"eligible,omitempty"`
	// EligibilityReaons is included when Eligible is false, and indicates the
	// reasons why this tier is not availble to the user.
	EligibilityReasons []string `json:"eligibility_reasons,omitempty"`
}

// RegistrySubscription is a user's subscription.
type RegistrySubscription struct {
	Tier      *RegistrySubscriptionTier `json:"tier"`
	CreatedAt time.Time                 `json:"created_at"`
	UpdatedAt time.Time                 `json:"updated_at"`
}

type registrySubscriptionRoot struct {
	Subscription *RegistrySubscription `json:"subscription"`
}

// RegistrySubscriptionUpdateRequest represents a request to update the
// subscription plan for a registry.
type RegistrySubscriptionUpdateRequest struct {
	TierSlug string `json:"tier_slug"`
}

// Get retrieves the details of a Registry.
func (svc *RegistryServiceOp) Get(ctx context.Context) (*Registry, *Response, error) {
	req, err := svc.client.NewRequest(ctx, http.MethodGet, registryPath, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(registryRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Registry, resp, nil
}

// Create creates a registry.
func (svc *RegistryServiceOp) Create(ctx context.Context, create *RegistryCreateRequest) (*Registry, *Response, error) {
	req, err := svc.client.NewRequest(ctx, http.MethodPost, registryPath, create)
	if err != nil {
		return nil, nil, err
	}
	root := new(registryRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Registry, resp, nil
}

// Delete deletes a registry. There is no way to recover a registry once it has
// been destroyed.
func (svc *RegistryServiceOp) Delete(ctx context.Context) (*Response, error) {
	req, err := svc.client.NewRequest(ctx, http.MethodDelete, registryPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := svc.client.Do(ctx, req, nil)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// DockerCredentials is the content of a Docker config file
// that is used by the docker CLI
// See: https://docs.docker.com/engine/reference/commandline/cli/#configjson-properties
type DockerCredentials struct {
	DockerConfigJSON []byte
}

// DockerCredentials retrieves a Docker config file containing the registry's credentials.
func (svc *RegistryServiceOp) DockerCredentials(ctx context.Context, request *RegistryDockerCredentialsRequest) (*DockerCredentials, *Response, error) {
	path := fmt.Sprintf("%s/%s", registryPath, "docker-credentials")
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	q := req.URL.Query()
	q.Add("read_write", strconv.FormatBool(request.ReadWrite))
	if request.ExpirySeconds != nil {
		q.Add("expiry_seconds", strconv.Itoa(*request.ExpirySeconds))
	}
	req.URL.RawQuery = q.Encode()

	var buf bytes.Buffer
	resp, err := svc.client.Do(ctx, req, &buf)
	if err != nil {
		return nil, resp, err
	}

	dc := &DockerCredentials{
		DockerConfigJSON: buf.Bytes(),
	}
	return dc, resp, nil
}

// ListRepositories returns a list of the Repositories visible with the registry's credentials.
func (svc *RegistryServiceOp) ListRepositories(ctx context.Context, registry string, opts *ListOptions) ([]*Repository, *Response, error) {
	path := fmt.Sprintf("%s/%s/repositories", registryPath, registry)
	path, err := addOptions(path, opts)
	if err != nil {
		return nil, nil, err
	}
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(repositoriesRoot)

	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	if l := root.Links; l != nil {
		resp.Links = l
	}
	if m := root.Meta; m != nil {
		resp.Meta = m
	}

	return root.Repositories, resp, nil
}

// ListRepositoryTags returns a list of the RepositoryTags available within the given repository.
func (svc *RegistryServiceOp) ListRepositoryTags(ctx context.Context, registry, repository string, opts *ListOptions) ([]*RepositoryTag, *Response, error) {
	path := fmt.Sprintf("%s/%s/repositories/%s/tags", registryPath, registry, url.PathEscape(repository))
	path, err := addOptions(path, opts)
	if err != nil {
		return nil, nil, err
	}
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(repositoryTagsRoot)

	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	if l := root.Links; l != nil {
		resp.Links = l
	}
	if m := root.Meta; m != nil {
		resp.Meta = m
	}

	return root.Tags, resp, nil
}

// DeleteTag deletes a tag within a given repository.
func (svc *RegistryServiceOp) DeleteTag(ctx context.Context, registry, repository, tag string) (*Response, error) {
	path := fmt.Sprintf("%s/%s/repositories/%s/tags/%s", registryPath, registry, url.PathEscape(repository), tag)
	req, err := svc.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := svc.client.Do(ctx, req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteManifest deletes a manifest by its digest within a given repository.
func (svc *RegistryServiceOp) DeleteManifest(ctx context.Context, registry, repository, digest string) (*Response, error) {
	path := fmt.Sprintf("%s/%s/repositories/%s/digests/%s", registryPath, registry, url.PathEscape(repository), digest)
	req, err := svc.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := svc.client.Do(ctx, req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

// StartGarbageCollection requests a garbage collection for the specified
// registry.
func (svc *RegistryServiceOp) StartGarbageCollection(ctx context.Context, registry string) (*GarbageCollection, *Response, error) {
	path := fmt.Sprintf("%s/%s/garbage-collection", registryPath, registry)
	req, err := svc.client.NewRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(garbageCollectionRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.GarbageCollection, resp, err
}

// GetGarbageCollection retrieves the currently-active garbage collection for
// the specified registry; if there are no active garbage collections, then
// return a 404/NotFound error. There can only be one active garbage
// collection on a registry.
func (svc *RegistryServiceOp) GetGarbageCollection(ctx context.Context, registry string) (*GarbageCollection, *Response, error) {
	path := fmt.Sprintf("%s/%s/garbage-collection", registryPath, registry)
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(garbageCollectionRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.GarbageCollection, resp, nil
}

// ListGarbageCollections retrieves all garbage collections (active and
// inactive) for the specified registry.
func (svc *RegistryServiceOp) ListGarbageCollections(ctx context.Context, registry string, opts *ListOptions) ([]*GarbageCollection, *Response, error) {
	path := fmt.Sprintf("%s/%s/garbage-collections", registryPath, registry)
	path, err := addOptions(path, opts)
	if err != nil {
		return nil, nil, err
	}
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(garbageCollectionsRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	if root.Links != nil {
		resp.Links = root.Links
	}
	if root.Meta != nil {
		resp.Meta = root.Meta
	}

	return root.GarbageCollections, resp, nil
}

// UpdateGarbageCollection updates the specified garbage collection for the
// specified registry. While only the currently-active garbage collection can
// be updated we still require the exact garbage collection to be specified to
// avoid race conditions that might may arise from issuing an update to the
// implicit "currently-active" garbage collection. Returns the updated garbage
// collection.
func (svc *RegistryServiceOp) UpdateGarbageCollection(ctx context.Context, registry, gcUUID string, request *UpdateGarbageCollectionRequest) (*GarbageCollection, *Response, error) {
	path := fmt.Sprintf("%s/%s/garbage-collection/%s", registryPath, registry, gcUUID)
	req, err := svc.client.NewRequest(ctx, http.MethodPut, path, request)
	if err != nil {
		return nil, nil, err
	}

	root := new(garbageCollectionRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.GarbageCollection, resp, nil
}

// GetOptions returns options the user can use when creating or updating a
// registry.
func (svc *RegistryServiceOp) GetOptions(ctx context.Context) (*RegistryOptions, *Response, error) {
	path := fmt.Sprintf("%s/options", registryPath)
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(registryOptionsRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Options, resp, nil
}

// GetSubscription retrieves the user's subscription.
func (svc *RegistryServiceOp) GetSubscription(ctx context.Context) (*RegistrySubscription, *Response, error) {
	path := fmt.Sprintf("%s/subscription", registryPath)
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(registrySubscriptionRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Subscription, resp, nil
}

// UpdateSubscription updates the user's registry subscription.
func (svc *RegistryServiceOp) UpdateSubscription(ctx context.Context, request *RegistrySubscriptionUpdateRequest) (*RegistrySubscription, *Response, error) {
	path := fmt.Sprintf("%s/subscription", registryPath)
	req, err := svc.client.NewRequest(ctx, http.MethodPost, path, request)
	if err != nil {
		return nil, nil, err
	}
	root := new(registrySubscriptionRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Subscription, resp, nil
}
