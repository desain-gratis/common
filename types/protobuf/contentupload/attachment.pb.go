// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.19.1
// source: contentupload/attachment.proto

package contentupload

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Or actual "content" that we uploaded to storage (eg. GCS); here is the
// meetadata.
// TODO: make generic in other module
type Attachment struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id           string   `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	RefId        string   `protobuf:"bytes,2,opt,name=ref_id,json=refId,proto3" json:"ref_id,omitempty"`
	OwnerId      string   `protobuf:"bytes,3,opt,name=owner_id,json=ownerId,proto3" json:"owner_id,omitempty"`
	Path         string   `protobuf:"bytes,4,opt,name=path,proto3" json:"path,omitempty"` // private path of the resource
	Name         string   `protobuf:"bytes,5,opt,name=name,proto3" json:"name,omitempty"` // name of the resource
	Url          string   `protobuf:"bytes,6,opt,name=url,proto3" json:"url,omitempty"`   // public URL of the resource
	ContentType  string   `protobuf:"bytes,7,opt,name=content_type,json=contentType,proto3" json:"content_type,omitempty"`
	ContentSize  int64    `protobuf:"varint,8,opt,name=content_size,json=contentSize,proto3" json:"content_size,omitempty"`
	Description  string   `protobuf:"bytes,9,opt,name=description,proto3" json:"description,omitempty"`
	Tags         []string `protobuf:"bytes,10,rep,name=tags,proto3" json:"tags,omitempty"` // meta data
	Ordering     int32    `protobuf:"varint,11,opt,name=ordering,proto3" json:"ordering,omitempty"`
	ImageDataUrl string   `protobuf:"bytes,12,opt,name=image_data_url,json=imageDataUrl,proto3" json:"image_data_url,omitempty"` // image (thumbnail) data URL if applicable
	CreatedAt    string   `protobuf:"bytes,13,opt,name=created_at,json=createdAt,proto3" json:"created_at,omitempty"`
	Hash         string   `protobuf:"bytes,14,opt,name=hash,proto3" json:"hash,omitempty"` // hash of the attachment
}

func (x *Attachment) Reset() {
	*x = Attachment{}
	if protoimpl.UnsafeEnabled {
		mi := &file_contentupload_attachment_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Attachment) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Attachment) ProtoMessage() {}

func (x *Attachment) ProtoReflect() protoreflect.Message {
	mi := &file_contentupload_attachment_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Attachment.ProtoReflect.Descriptor instead.
func (*Attachment) Descriptor() ([]byte, []int) {
	return file_contentupload_attachment_proto_rawDescGZIP(), []int{0}
}

func (x *Attachment) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *Attachment) GetRefId() string {
	if x != nil {
		return x.RefId
	}
	return ""
}

func (x *Attachment) GetOwnerId() string {
	if x != nil {
		return x.OwnerId
	}
	return ""
}

func (x *Attachment) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

func (x *Attachment) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Attachment) GetUrl() string {
	if x != nil {
		return x.Url
	}
	return ""
}

func (x *Attachment) GetContentType() string {
	if x != nil {
		return x.ContentType
	}
	return ""
}

func (x *Attachment) GetContentSize() int64 {
	if x != nil {
		return x.ContentSize
	}
	return 0
}

func (x *Attachment) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *Attachment) GetTags() []string {
	if x != nil {
		return x.Tags
	}
	return nil
}

func (x *Attachment) GetOrdering() int32 {
	if x != nil {
		return x.Ordering
	}
	return 0
}

func (x *Attachment) GetImageDataUrl() string {
	if x != nil {
		return x.ImageDataUrl
	}
	return ""
}

func (x *Attachment) GetCreatedAt() string {
	if x != nil {
		return x.CreatedAt
	}
	return ""
}

func (x *Attachment) GetHash() string {
	if x != nil {
		return x.Hash
	}
	return ""
}

var File_contentupload_attachment_proto protoreflect.FileDescriptor

var file_contentupload_attachment_proto_rawDesc = []byte{
	0x0a, 0x1e, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x75, 0x70, 0x6c, 0x6f, 0x61, 0x64, 0x2f,
	0x61, 0x74, 0x74, 0x61, 0x63, 0x68, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x12, 0x0d, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x75, 0x70, 0x6c, 0x6f, 0x61, 0x64, 0x22,
	0xf9, 0x02, 0x0a, 0x0a, 0x41, 0x74, 0x74, 0x61, 0x63, 0x68, 0x6d, 0x65, 0x6e, 0x74, 0x12, 0x0e,
	0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x15,
	0x0a, 0x06, 0x72, 0x65, 0x66, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05,
	0x72, 0x65, 0x66, 0x49, 0x64, 0x12, 0x19, 0x0a, 0x08, 0x6f, 0x77, 0x6e, 0x65, 0x72, 0x5f, 0x69,
	0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6f, 0x77, 0x6e, 0x65, 0x72, 0x49, 0x64,
	0x12, 0x12, 0x0a, 0x04, 0x70, 0x61, 0x74, 0x68, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x70, 0x61, 0x74, 0x68, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x05, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x72, 0x6c, 0x18,
	0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x75, 0x72, 0x6c, 0x12, 0x21, 0x0a, 0x0c, 0x63, 0x6f,
	0x6e, 0x74, 0x65, 0x6e, 0x74, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0b, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x54, 0x79, 0x70, 0x65, 0x12, 0x21, 0x0a,
	0x0c, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x08, 0x20,
	0x01, 0x28, 0x03, 0x52, 0x0b, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x53, 0x69, 0x7a, 0x65,
	0x12, 0x20, 0x0a, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18,
	0x09, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69,
	0x6f, 0x6e, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x61, 0x67, 0x73, 0x18, 0x0a, 0x20, 0x03, 0x28, 0x09,
	0x52, 0x04, 0x74, 0x61, 0x67, 0x73, 0x12, 0x1a, 0x0a, 0x08, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x69,
	0x6e, 0x67, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x05, 0x52, 0x08, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x69,
	0x6e, 0x67, 0x12, 0x24, 0x0a, 0x0e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x5f, 0x64, 0x61, 0x74, 0x61,
	0x5f, 0x75, 0x72, 0x6c, 0x18, 0x0c, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x69, 0x6d, 0x61, 0x67,
	0x65, 0x44, 0x61, 0x74, 0x61, 0x55, 0x72, 0x6c, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x72, 0x65, 0x61,
	0x74, 0x65, 0x64, 0x5f, 0x61, 0x74, 0x18, 0x0d, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x63, 0x72,
	0x65, 0x61, 0x74, 0x65, 0x64, 0x41, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x68, 0x61, 0x73, 0x68, 0x18,
	0x0e, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x68, 0x61, 0x73, 0x68, 0x42, 0x4c, 0x5a, 0x4a, 0x67,
	0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x64, 0x65, 0x73, 0x61, 0x69, 0x6e,
	0x2d, 0x67, 0x72, 0x61, 0x74, 0x69, 0x73, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2f, 0x74,
	0x79, 0x70, 0x65, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x63, 0x6f,
	0x6e, 0x74, 0x65, 0x6e, 0x74, 0x75, 0x70, 0x6c, 0x6f, 0x61, 0x64, 0x3b, 0x63, 0x6f, 0x6e, 0x74,
	0x65, 0x6e, 0x74, 0x75, 0x70, 0x6c, 0x6f, 0x61, 0x64, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_contentupload_attachment_proto_rawDescOnce sync.Once
	file_contentupload_attachment_proto_rawDescData = file_contentupload_attachment_proto_rawDesc
)

func file_contentupload_attachment_proto_rawDescGZIP() []byte {
	file_contentupload_attachment_proto_rawDescOnce.Do(func() {
		file_contentupload_attachment_proto_rawDescData = protoimpl.X.CompressGZIP(file_contentupload_attachment_proto_rawDescData)
	})
	return file_contentupload_attachment_proto_rawDescData
}

var file_contentupload_attachment_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_contentupload_attachment_proto_goTypes = []interface{}{
	(*Attachment)(nil), // 0: contentupload.Attachment
}
var file_contentupload_attachment_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_contentupload_attachment_proto_init() }
func file_contentupload_attachment_proto_init() {
	if File_contentupload_attachment_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_contentupload_attachment_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Attachment); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_contentupload_attachment_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_contentupload_attachment_proto_goTypes,
		DependencyIndexes: file_contentupload_attachment_proto_depIdxs,
		MessageInfos:      file_contentupload_attachment_proto_msgTypes,
	}.Build()
	File_contentupload_attachment_proto = out.File
	file_contentupload_attachment_proto_rawDesc = nil
	file_contentupload_attachment_proto_goTypes = nil
	file_contentupload_attachment_proto_depIdxs = nil
}
