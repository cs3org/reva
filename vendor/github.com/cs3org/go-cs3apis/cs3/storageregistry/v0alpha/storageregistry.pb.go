// Code generated by protoc-gen-go. DO NOT EDIT.
// source: cs3/storageregistry/v0alpha/storageregistry.proto

package storageregistryv0alphapb

import (
	context "context"
	fmt "fmt"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc"
	v0alpha "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storagetypes "github.com/cs3org/go-cs3apis/cs3/storagetypes"
	types "github.com/cs3org/go-cs3apis/cs3/types"
	proto "github.com/golang/protobuf/proto"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type GetStorageProviderRequest struct {
	// OPTIONAL.
	// Opaque information.
	Opaque *types.Opaque `protobuf:"bytes,1,opt,name=opaque,proto3" json:"opaque,omitempty"`
	// REQUIRED.
	// The reference for the resource.
	Ref                  *v0alpha.Reference `protobuf:"bytes,2,opt,name=ref,proto3" json:"ref,omitempty"`
	XXX_NoUnkeyedLiteral struct{}           `json:"-"`
	XXX_unrecognized     []byte             `json:"-"`
	XXX_sizecache        int32              `json:"-"`
}

func (m *GetStorageProviderRequest) Reset()         { *m = GetStorageProviderRequest{} }
func (m *GetStorageProviderRequest) String() string { return proto.CompactTextString(m) }
func (*GetStorageProviderRequest) ProtoMessage()    {}
func (*GetStorageProviderRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_28eee549603e3a86, []int{0}
}

func (m *GetStorageProviderRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetStorageProviderRequest.Unmarshal(m, b)
}
func (m *GetStorageProviderRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetStorageProviderRequest.Marshal(b, m, deterministic)
}
func (m *GetStorageProviderRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetStorageProviderRequest.Merge(m, src)
}
func (m *GetStorageProviderRequest) XXX_Size() int {
	return xxx_messageInfo_GetStorageProviderRequest.Size(m)
}
func (m *GetStorageProviderRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetStorageProviderRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetStorageProviderRequest proto.InternalMessageInfo

func (m *GetStorageProviderRequest) GetOpaque() *types.Opaque {
	if m != nil {
		return m.Opaque
	}
	return nil
}

func (m *GetStorageProviderRequest) GetRef() *v0alpha.Reference {
	if m != nil {
		return m.Ref
	}
	return nil
}

type GetStorageProviderResponse struct {
	// REQUIRED.
	// The response status.
	Status *rpc.Status `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	// OPTIONAL.
	// Opaque information.
	Opaque *types.Opaque `protobuf:"bytes,2,opt,name=opaque,proto3" json:"opaque,omitempty"`
	// REQUIRED.
	// The storage provider handling the requested storage resource.
	Provider             *storagetypes.ProviderInfo `protobuf:"bytes,3,opt,name=provider,proto3" json:"provider,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                   `json:"-"`
	XXX_unrecognized     []byte                     `json:"-"`
	XXX_sizecache        int32                      `json:"-"`
}

func (m *GetStorageProviderResponse) Reset()         { *m = GetStorageProviderResponse{} }
func (m *GetStorageProviderResponse) String() string { return proto.CompactTextString(m) }
func (*GetStorageProviderResponse) ProtoMessage()    {}
func (*GetStorageProviderResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_28eee549603e3a86, []int{1}
}

func (m *GetStorageProviderResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetStorageProviderResponse.Unmarshal(m, b)
}
func (m *GetStorageProviderResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetStorageProviderResponse.Marshal(b, m, deterministic)
}
func (m *GetStorageProviderResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetStorageProviderResponse.Merge(m, src)
}
func (m *GetStorageProviderResponse) XXX_Size() int {
	return xxx_messageInfo_GetStorageProviderResponse.Size(m)
}
func (m *GetStorageProviderResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_GetStorageProviderResponse.DiscardUnknown(m)
}

var xxx_messageInfo_GetStorageProviderResponse proto.InternalMessageInfo

func (m *GetStorageProviderResponse) GetStatus() *rpc.Status {
	if m != nil {
		return m.Status
	}
	return nil
}

func (m *GetStorageProviderResponse) GetOpaque() *types.Opaque {
	if m != nil {
		return m.Opaque
	}
	return nil
}

func (m *GetStorageProviderResponse) GetProvider() *storagetypes.ProviderInfo {
	if m != nil {
		return m.Provider
	}
	return nil
}

type ListStorageProvidersRequest struct {
	// OPTIONAL.
	// Opaque information.
	Opaque               *types.Opaque `protobuf:"bytes,1,opt,name=opaque,proto3" json:"opaque,omitempty"`
	XXX_NoUnkeyedLiteral struct{}      `json:"-"`
	XXX_unrecognized     []byte        `json:"-"`
	XXX_sizecache        int32         `json:"-"`
}

func (m *ListStorageProvidersRequest) Reset()         { *m = ListStorageProvidersRequest{} }
func (m *ListStorageProvidersRequest) String() string { return proto.CompactTextString(m) }
func (*ListStorageProvidersRequest) ProtoMessage()    {}
func (*ListStorageProvidersRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_28eee549603e3a86, []int{2}
}

func (m *ListStorageProvidersRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListStorageProvidersRequest.Unmarshal(m, b)
}
func (m *ListStorageProvidersRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListStorageProvidersRequest.Marshal(b, m, deterministic)
}
func (m *ListStorageProvidersRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListStorageProvidersRequest.Merge(m, src)
}
func (m *ListStorageProvidersRequest) XXX_Size() int {
	return xxx_messageInfo_ListStorageProvidersRequest.Size(m)
}
func (m *ListStorageProvidersRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ListStorageProvidersRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ListStorageProvidersRequest proto.InternalMessageInfo

func (m *ListStorageProvidersRequest) GetOpaque() *types.Opaque {
	if m != nil {
		return m.Opaque
	}
	return nil
}

type ListStorageProvidersResponse struct {
	// REQUIRED.
	// The response status.
	Status *rpc.Status `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	// OPTIONAL.
	// Opaque information.
	Opaque *types.Opaque `protobuf:"bytes,2,opt,name=opaque,proto3" json:"opaque,omitempty"`
	// REQUIRED.
	// The list of storage providers this registry knows about.
	Providers            []*storagetypes.ProviderInfo `protobuf:"bytes,3,rep,name=providers,proto3" json:"providers,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                     `json:"-"`
	XXX_unrecognized     []byte                       `json:"-"`
	XXX_sizecache        int32                        `json:"-"`
}

func (m *ListStorageProvidersResponse) Reset()         { *m = ListStorageProvidersResponse{} }
func (m *ListStorageProvidersResponse) String() string { return proto.CompactTextString(m) }
func (*ListStorageProvidersResponse) ProtoMessage()    {}
func (*ListStorageProvidersResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_28eee549603e3a86, []int{3}
}

func (m *ListStorageProvidersResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListStorageProvidersResponse.Unmarshal(m, b)
}
func (m *ListStorageProvidersResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListStorageProvidersResponse.Marshal(b, m, deterministic)
}
func (m *ListStorageProvidersResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListStorageProvidersResponse.Merge(m, src)
}
func (m *ListStorageProvidersResponse) XXX_Size() int {
	return xxx_messageInfo_ListStorageProvidersResponse.Size(m)
}
func (m *ListStorageProvidersResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ListStorageProvidersResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ListStorageProvidersResponse proto.InternalMessageInfo

func (m *ListStorageProvidersResponse) GetStatus() *rpc.Status {
	if m != nil {
		return m.Status
	}
	return nil
}

func (m *ListStorageProvidersResponse) GetOpaque() *types.Opaque {
	if m != nil {
		return m.Opaque
	}
	return nil
}

func (m *ListStorageProvidersResponse) GetProviders() []*storagetypes.ProviderInfo {
	if m != nil {
		return m.Providers
	}
	return nil
}

type GetHomeRequest struct {
	// OPTIONAL.
	// Opaque information.
	Opaque               *types.Opaque `protobuf:"bytes,1,opt,name=opaque,proto3" json:"opaque,omitempty"`
	XXX_NoUnkeyedLiteral struct{}      `json:"-"`
	XXX_unrecognized     []byte        `json:"-"`
	XXX_sizecache        int32         `json:"-"`
}

func (m *GetHomeRequest) Reset()         { *m = GetHomeRequest{} }
func (m *GetHomeRequest) String() string { return proto.CompactTextString(m) }
func (*GetHomeRequest) ProtoMessage()    {}
func (*GetHomeRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_28eee549603e3a86, []int{4}
}

func (m *GetHomeRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetHomeRequest.Unmarshal(m, b)
}
func (m *GetHomeRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetHomeRequest.Marshal(b, m, deterministic)
}
func (m *GetHomeRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetHomeRequest.Merge(m, src)
}
func (m *GetHomeRequest) XXX_Size() int {
	return xxx_messageInfo_GetHomeRequest.Size(m)
}
func (m *GetHomeRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetHomeRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetHomeRequest proto.InternalMessageInfo

func (m *GetHomeRequest) GetOpaque() *types.Opaque {
	if m != nil {
		return m.Opaque
	}
	return nil
}

type GetHomeResponse struct {
	// REQUIRED.
	// The response status.
	Status *rpc.Status `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	// OPTIONAL.
	// Opaque information.
	Opaque *types.Opaque `protobuf:"bytes,2,opt,name=opaque,proto3" json:"opaque,omitempty"`
	// REQUIRED.
	// The path to the home in a storage provider.
	// For example /eos/user/h/hugo in the storage provider with root path /eos/user/.
	Path                 string   `protobuf:"bytes,3,opt,name=path,proto3" json:"path,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetHomeResponse) Reset()         { *m = GetHomeResponse{} }
func (m *GetHomeResponse) String() string { return proto.CompactTextString(m) }
func (*GetHomeResponse) ProtoMessage()    {}
func (*GetHomeResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_28eee549603e3a86, []int{5}
}

func (m *GetHomeResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetHomeResponse.Unmarshal(m, b)
}
func (m *GetHomeResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetHomeResponse.Marshal(b, m, deterministic)
}
func (m *GetHomeResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetHomeResponse.Merge(m, src)
}
func (m *GetHomeResponse) XXX_Size() int {
	return xxx_messageInfo_GetHomeResponse.Size(m)
}
func (m *GetHomeResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_GetHomeResponse.DiscardUnknown(m)
}

var xxx_messageInfo_GetHomeResponse proto.InternalMessageInfo

func (m *GetHomeResponse) GetStatus() *rpc.Status {
	if m != nil {
		return m.Status
	}
	return nil
}

func (m *GetHomeResponse) GetOpaque() *types.Opaque {
	if m != nil {
		return m.Opaque
	}
	return nil
}

func (m *GetHomeResponse) GetPath() string {
	if m != nil {
		return m.Path
	}
	return ""
}

func init() {
	proto.RegisterType((*GetStorageProviderRequest)(nil), "cs3.storageregistryv0alpha.GetStorageProviderRequest")
	proto.RegisterType((*GetStorageProviderResponse)(nil), "cs3.storageregistryv0alpha.GetStorageProviderResponse")
	proto.RegisterType((*ListStorageProvidersRequest)(nil), "cs3.storageregistryv0alpha.ListStorageProvidersRequest")
	proto.RegisterType((*ListStorageProvidersResponse)(nil), "cs3.storageregistryv0alpha.ListStorageProvidersResponse")
	proto.RegisterType((*GetHomeRequest)(nil), "cs3.storageregistryv0alpha.GetHomeRequest")
	proto.RegisterType((*GetHomeResponse)(nil), "cs3.storageregistryv0alpha.GetHomeResponse")
}

func init() {
	proto.RegisterFile("cs3/storageregistry/v0alpha/storageregistry.proto", fileDescriptor_28eee549603e3a86)
}

var fileDescriptor_28eee549603e3a86 = []byte{
	// 498 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xb4, 0x54, 0xcf, 0x6b, 0x13, 0x41,
	0x14, 0x66, 0xb3, 0x12, 0xed, 0x13, 0x2c, 0x8e, 0x51, 0xd6, 0x55, 0x4a, 0x59, 0x11, 0x7f, 0x14,
	0x26, 0xb5, 0x41, 0x2b, 0xea, 0xa5, 0x29, 0x25, 0x2d, 0x08, 0x09, 0xb3, 0x45, 0x54, 0xbc, 0x6c,
	0xc6, 0x97, 0x36, 0x60, 0x33, 0xd3, 0x99, 0x49, 0x20, 0x27, 0x0f, 0x9e, 0xbc, 0xf8, 0x47, 0xf4,
	0x28, 0x1e, 0xfc, 0x3b, 0xfc, 0xab, 0x64, 0x67, 0x67, 0xe3, 0x36, 0x6e, 0xd3, 0xa6, 0x90, 0xdb,
	0xee, 0x7b, 0xdf, 0xf7, 0xde, 0xf7, 0xbd, 0x79, 0x33, 0xf0, 0x8c, 0xeb, 0x46, 0x5d, 0x1b, 0xa1,
	0x92, 0x03, 0x54, 0x78, 0xd0, 0xd7, 0x46, 0x8d, 0xeb, 0xa3, 0xf5, 0xe4, 0x8b, 0x3c, 0x4c, 0xa6,
	0xe3, 0x54, 0x2a, 0x61, 0x04, 0x09, 0xb9, 0x6e, 0xd0, 0xa9, 0x94, 0x63, 0x84, 0xb5, 0xb4, 0x9c,
	0x92, 0xbc, 0xae, 0x4d, 0x62, 0x86, 0x3a, 0x63, 0x84, 0x6b, 0x85, 0x26, 0x52, 0x89, 0x51, 0xff,
	0x33, 0xaa, 0x49, 0x13, 0x85, 0x5a, 0x0c, 0x15, 0xc7, 0x1c, 0xfc, 0xa0, 0x00, 0x36, 0x63, 0x89,
	0xfa, 0xd4, 0x8f, 0x03, 0xdd, 0x4e, 0x41, 0x59, 0xb6, 0x10, 0x8e, 0xbe, 0xc2, 0xdd, 0x16, 0x9a,
	0x38, 0xc3, 0x77, 0x5c, 0x27, 0x86, 0xc7, 0x43, 0xd4, 0x86, 0x3c, 0x81, 0xaa, 0x90, 0xc9, 0xf1,
	0x10, 0x03, 0x6f, 0xd5, 0x7b, 0x7c, 0x7d, 0xe3, 0x26, 0x4d, 0x8d, 0x64, 0xf4, 0xb6, 0x4d, 0x30,
	0x07, 0x20, 0x9b, 0xe0, 0x2b, 0xec, 0x05, 0x15, 0x8b, 0x7b, 0x48, 0x0b, 0x86, 0x73, 0xf9, 0x4e,
	0x3d, 0x65, 0xd8, 0x43, 0x85, 0x03, 0x8e, 0x2c, 0x65, 0x44, 0xbf, 0x3c, 0x08, 0xcb, 0x14, 0x68,
	0x29, 0x06, 0x1a, 0xc9, 0x23, 0xa8, 0x66, 0x83, 0x71, 0x12, 0x96, 0x6d, 0x69, 0x25, 0x39, 0x8d,
	0x6d, 0x98, 0xb9, 0x74, 0x41, 0x6b, 0xe5, 0x3c, 0xad, 0xaf, 0xe0, 0x5a, 0x2e, 0x2a, 0xf0, 0x2d,
	0x78, 0xa5, 0x28, 0x38, 0xe3, 0xe4, 0x4a, 0xf6, 0x06, 0x3d, 0xc1, 0x26, 0xf8, 0x68, 0x17, 0xee,
	0xbd, 0xed, 0xeb, 0x69, 0xb9, 0x7a, 0xfe, 0x89, 0x45, 0xbf, 0x3d, 0xb8, 0x5f, 0x5e, 0x6a, 0x81,
	0xd6, 0xdf, 0xc0, 0x52, 0x6e, 0x45, 0x07, 0xfe, 0xaa, 0x7f, 0x01, 0xef, 0xff, 0x08, 0xd1, 0x6b,
	0xb8, 0xd1, 0x42, 0xb3, 0x2b, 0x8e, 0xf0, 0x12, 0x7e, 0xc7, 0xb0, 0x3c, 0x21, 0x2f, 0xd0, 0x21,
	0x81, 0x2b, 0x32, 0x31, 0x87, 0xf6, 0x60, 0x97, 0x98, 0xfd, 0xde, 0xf8, 0xe1, 0xc3, 0x1d, 0x37,
	0x66, 0xe6, 0xae, 0x5f, 0x8c, 0x6a, 0xd4, 0xe7, 0x48, 0xbe, 0x79, 0x40, 0xfe, 0x5f, 0x3f, 0xf2,
	0x9c, 0x9e, 0x7d, 0x65, 0xe9, 0x99, 0x17, 0x26, 0x7c, 0x31, 0x2f, 0xcd, 0x0d, 0xe2, 0xbb, 0x07,
	0xb5, 0xb2, 0x5d, 0x20, 0x9b, 0xb3, 0x0a, 0xce, 0x58, 0xc4, 0xf0, 0xe5, 0xfc, 0x44, 0xa7, 0xa5,
	0x0b, 0x57, 0xdd, 0x39, 0x91, 0xa7, 0xe7, 0xd8, 0x29, 0x6c, 0x42, 0xb8, 0x76, 0x21, 0x6c, 0xd6,
	0xa3, 0x79, 0xe2, 0xc1, 0x0a, 0x17, 0x47, 0x33, 0x28, 0xcd, 0x5a, 0x7c, 0x3a, 0xde, 0x49, 0x9f,
	0xab, 0x8e, 0xf7, 0x31, 0x28, 0xc7, 0xcb, 0xee, 0x49, 0xe5, 0xd6, 0x76, 0xb3, 0xfd, 0x3e, 0xde,
	0x6f, 0xb3, 0xad, 0xd6, 0x0e, 0xdb, 0x69, 0xed, 0xc5, 0xfb, 0xec, 0xc3, 0xcf, 0x4a, 0xb8, 0x1d,
	0x37, 0xe8, 0xd4, 0xe9, 0xbf, 0x5b, 0xdf, 0x4a, 0x49, 0x7f, 0x6c, 0xf2, 0x53, 0x79, 0xb2, 0x5b,
	0xb5, 0x2f, 0x64, 0xe3, 0x6f, 0x00, 0x00, 0x00, 0xff, 0xff, 0xb1, 0x53, 0x43, 0x3a, 0xf1, 0x05,
	0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// StorageRegistryServiceClient is the client API for StorageRegistryService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type StorageRegistryServiceClient interface {
	// Returns the storage provider that is reponsible for the given
	// resource reference.
	// MUST return CODE_NOT_FOUND if the reference does not exist.
	GetStorageProvider(ctx context.Context, in *GetStorageProviderRequest, opts ...grpc.CallOption) (*GetStorageProviderResponse, error)
	// Returns a list of the available storage providers known by this registry.
	ListStorageProviders(ctx context.Context, in *ListStorageProvidersRequest, opts ...grpc.CallOption) (*ListStorageProvidersResponse, error)
	// Returns the home path for the given authenticated user.
	// When a user has access to multiple storage providers, one of them is the home.
	GetHome(ctx context.Context, in *GetHomeRequest, opts ...grpc.CallOption) (*GetHomeResponse, error)
}

type storageRegistryServiceClient struct {
	cc *grpc.ClientConn
}

func NewStorageRegistryServiceClient(cc *grpc.ClientConn) StorageRegistryServiceClient {
	return &storageRegistryServiceClient{cc}
}

func (c *storageRegistryServiceClient) GetStorageProvider(ctx context.Context, in *GetStorageProviderRequest, opts ...grpc.CallOption) (*GetStorageProviderResponse, error) {
	out := new(GetStorageProviderResponse)
	err := c.cc.Invoke(ctx, "/cs3.storageregistryv0alpha.StorageRegistryService/GetStorageProvider", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storageRegistryServiceClient) ListStorageProviders(ctx context.Context, in *ListStorageProvidersRequest, opts ...grpc.CallOption) (*ListStorageProvidersResponse, error) {
	out := new(ListStorageProvidersResponse)
	err := c.cc.Invoke(ctx, "/cs3.storageregistryv0alpha.StorageRegistryService/ListStorageProviders", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storageRegistryServiceClient) GetHome(ctx context.Context, in *GetHomeRequest, opts ...grpc.CallOption) (*GetHomeResponse, error) {
	out := new(GetHomeResponse)
	err := c.cc.Invoke(ctx, "/cs3.storageregistryv0alpha.StorageRegistryService/GetHome", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// StorageRegistryServiceServer is the server API for StorageRegistryService service.
type StorageRegistryServiceServer interface {
	// Returns the storage provider that is reponsible for the given
	// resource reference.
	// MUST return CODE_NOT_FOUND if the reference does not exist.
	GetStorageProvider(context.Context, *GetStorageProviderRequest) (*GetStorageProviderResponse, error)
	// Returns a list of the available storage providers known by this registry.
	ListStorageProviders(context.Context, *ListStorageProvidersRequest) (*ListStorageProvidersResponse, error)
	// Returns the home path for the given authenticated user.
	// When a user has access to multiple storage providers, one of them is the home.
	GetHome(context.Context, *GetHomeRequest) (*GetHomeResponse, error)
}

// UnimplementedStorageRegistryServiceServer can be embedded to have forward compatible implementations.
type UnimplementedStorageRegistryServiceServer struct {
}

func (*UnimplementedStorageRegistryServiceServer) GetStorageProvider(ctx context.Context, req *GetStorageProviderRequest) (*GetStorageProviderResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetStorageProvider not implemented")
}
func (*UnimplementedStorageRegistryServiceServer) ListStorageProviders(ctx context.Context, req *ListStorageProvidersRequest) (*ListStorageProvidersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListStorageProviders not implemented")
}
func (*UnimplementedStorageRegistryServiceServer) GetHome(ctx context.Context, req *GetHomeRequest) (*GetHomeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetHome not implemented")
}

func RegisterStorageRegistryServiceServer(s *grpc.Server, srv StorageRegistryServiceServer) {
	s.RegisterService(&_StorageRegistryService_serviceDesc, srv)
}

func _StorageRegistryService_GetStorageProvider_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetStorageProviderRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageRegistryServiceServer).GetStorageProvider(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/cs3.storageregistryv0alpha.StorageRegistryService/GetStorageProvider",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageRegistryServiceServer).GetStorageProvider(ctx, req.(*GetStorageProviderRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorageRegistryService_ListStorageProviders_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListStorageProvidersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageRegistryServiceServer).ListStorageProviders(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/cs3.storageregistryv0alpha.StorageRegistryService/ListStorageProviders",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageRegistryServiceServer).ListStorageProviders(ctx, req.(*ListStorageProvidersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorageRegistryService_GetHome_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetHomeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageRegistryServiceServer).GetHome(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/cs3.storageregistryv0alpha.StorageRegistryService/GetHome",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageRegistryServiceServer).GetHome(ctx, req.(*GetHomeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _StorageRegistryService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "cs3.storageregistryv0alpha.StorageRegistryService",
	HandlerType: (*StorageRegistryServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetStorageProvider",
			Handler:    _StorageRegistryService_GetStorageProvider_Handler,
		},
		{
			MethodName: "ListStorageProviders",
			Handler:    _StorageRegistryService_ListStorageProviders_Handler,
		},
		{
			MethodName: "GetHome",
			Handler:    _StorageRegistryService_GetHome_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "cs3/storageregistry/v0alpha/storageregistry.proto",
}
