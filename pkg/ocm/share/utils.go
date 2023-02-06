package share

import (
	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

func NewWebDAVProtocol(uri, shareSecred string, perms *ocm.SharePermissions) *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_WebdavOptions{
			WebdavOptions: &ocm.WebDAVProtocol{
				Uri:          uri,
				SharedSecret: shareSecred,
				Permissions:  perms,
			},
		},
	}
}

func NewWebappProtocol(uriTemplate string) *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_WebappOptions{
			WebappOptions: &ocm.WebappProtocol{
				UriTemplate: uriTemplate,
			},
		},
	}
}

func NewTransferProtocol(sourceUri, sharedSecret string, size uint64) *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_TransferOptions{
			TransferOptions: &ocm.TransferProtocol{
				SourceUri:    sourceUri,
				SharedSecret: sharedSecret,
				Size:         size,
			},
		},
	}
}

func NewWebDavAccessMethod(perms *provider.ResourcePermissions) *ocm.AccessMethod {
	return &ocm.AccessMethod{
		Term: &ocm.AccessMethod_WebdavOptions{
			WebdavOptions: &ocm.WebDAVAccessMethod{
				Permissions: perms,
			},
		},
	}
}

func NewWebappAccessMethod(mode appprovider.ViewMode) *ocm.AccessMethod {
	return &ocm.AccessMethod{
		Term: &ocm.AccessMethod_WebappOptions{
			WebappOptions: &ocm.WebappAccessMethod{
				ViewMode: mode,
			},
		},
	}
}

func NewTransferAccessMethod() *ocm.AccessMethod {
	return &ocm.AccessMethod{
		Term: &ocm.AccessMethod_TransferOptions{
			TransferOptions: &ocm.TransferAccessMethod{},
		},
	}
}
