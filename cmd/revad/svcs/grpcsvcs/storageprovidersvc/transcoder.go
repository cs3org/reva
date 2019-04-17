package storageprovidersvc

import (
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

// XS defines an hex-encoded string as checksum.
type XS string

const (
	// XSInvalid means the checksum type is invalid.
	XSInvalid XS = "invalid"
	//XSUnset means the checksum is optional.
	XSUnset = "unset"
	// XSAdler32 means the checkum is adler32
	XSAdler32 = "adler32"
	// XSMD5 means the checksum is md5
	XSMD5 = "md5"
	// XSSHA1 means the checksum is SHA1
	XSSHA1 = "sha1"
	// XSSHA256 means the checksum is SHA256.
	XSSHA256 = "sha256"
)

// GRPC2PKGXS converts the grpc checksum type to an internal pkg type.
func GRPC2PKGXS(t storageproviderv0alphapb.ResourceChecksumType) XS {
	switch t {
	case storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID:
		return XSInvalid
	case storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET:
		return XSUnset
	case storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1:
		return XSSHA1
	case storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32:
		return XSAdler32
	default:
		return XSInvalid
	}
}

// PKG2GRPCXS converts an internal checksum type to the grpc checksum type.
func PKG2GRPCXS(xsType string) storageproviderv0alphapb.ResourceChecksumType {
	switch xsType {
	case XSUnset:
		return storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET
	case XSAdler32:
		return storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32
	case XSMD5:
		return storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5
	case XSSHA1:
		return storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1
	default:
		return storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID
	}
}
