package spaces

import (
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type ListStorageSpaceFilter struct {
	filters []*providerpb.ListStorageSpacesRequest_Filter
}

func (f ListStorageSpaceFilter) ByID(id *providerpb.StorageSpaceId) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_ID,
		Term: &providerpb.ListStorageSpacesRequest_Filter_Id{
			Id: id,
		},
	})
	return f
}

func (f ListStorageSpaceFilter) ByOwner(owner *userpb.UserId) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_OWNER,
		Term: &providerpb.ListStorageSpacesRequest_Filter_Owner{
			Owner: owner,
		},
	})
	return f
}

func (f ListStorageSpaceFilter) BySpaceType(spaceType SpaceType) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
		Term: &providerpb.ListStorageSpacesRequest_Filter_SpaceType{
			SpaceType: spaceType.AsString(),
		},
	})
	return f
}

func (f ListStorageSpaceFilter) ByPath(path string) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_PATH,
		Term: &providerpb.ListStorageSpacesRequest_Filter_Path{
			Path: path,
		},
	})
	return f
}

func (f ListStorageSpaceFilter) ByUser(user *userpb.UserId) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_USER,
		Term: &providerpb.ListStorageSpacesRequest_Filter_User{
			User: user,
		},
	})
	return f
}

func (f ListStorageSpaceFilter) List() []*providerpb.ListStorageSpacesRequest_Filter {
	return f.filters
}
