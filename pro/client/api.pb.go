// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v4.24.3
// source: pro/client/api.proto

package client

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	structpb "google.golang.org/protobuf/types/known/structpb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Provider int32

const (
	Provider_PROVIDER_UNSET Provider = 0
	Provider_STRIPE         Provider = 1
	Provider_FREEKASSA      Provider = 2
)

// Enum value maps for Provider.
var (
	Provider_name = map[int32]string{
		0: "PROVIDER_UNSET",
		1: "STRIPE",
		2: "FREEKASSA",
	}
	Provider_value = map[string]int32{
		"PROVIDER_UNSET": 0,
		"STRIPE":         1,
		"FREEKASSA":      2,
	}
)

func (x Provider) Enum() *Provider {
	p := new(Provider)
	*p = x
	return p
}

func (x Provider) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Provider) Descriptor() protoreflect.EnumDescriptor {
	return file_pro_client_api_proto_enumTypes[0].Descriptor()
}

func (Provider) Type() protoreflect.EnumType {
	return &file_pro_client_api_proto_enumTypes[0]
}

func (x Provider) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Provider.Descriptor instead.
func (Provider) EnumDescriptor() ([]byte, []int) {
	return file_pro_client_api_proto_rawDescGZIP(), []int{0}
}

type Plan struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id                     string           `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Description            string           `protobuf:"bytes,2,opt,name=description,proto3" json:"description,omitempty"`
	BestValue              bool             `protobuf:"varint,3,opt,name=bestValue,proto3" json:"bestValue,omitempty"`
	UsdPrice               int64            `protobuf:"varint,4,opt,name=usdPrice,proto3" json:"usdPrice,omitempty"`
	Price                  map[string]int64 `protobuf:"bytes,5,rep,name=price,proto3" json:"price,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"varint,2,opt,name=value,proto3"`
	ExpectedMonthlyPrice   map[string]int64 `protobuf:"bytes,6,rep,name=expectedMonthlyPrice,proto3" json:"expectedMonthlyPrice,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"varint,2,opt,name=value,proto3"`
	TotalCostBilledOneTime string           `protobuf:"bytes,7,opt,name=totalCostBilledOneTime,proto3" json:"totalCostBilledOneTime,omitempty"`
	OneMonthCost           string           `protobuf:"bytes,8,opt,name=oneMonthCost,proto3" json:"oneMonthCost,omitempty"`
	TotalCost              string           `protobuf:"bytes,9,opt,name=totalCost,proto3" json:"totalCost,omitempty"`
	FormattedBonus         string           `protobuf:"bytes,10,opt,name=formattedBonus,proto3" json:"formattedBonus,omitempty"`
	RenewalText            string           `protobuf:"bytes,11,opt,name=renewalText,proto3" json:"renewalText,omitempty"`
}

func (x *Plan) Reset() {
	*x = Plan{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pro_client_api_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Plan) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Plan) ProtoMessage() {}

func (x *Plan) ProtoReflect() protoreflect.Message {
	mi := &file_pro_client_api_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Plan.ProtoReflect.Descriptor instead.
func (*Plan) Descriptor() ([]byte, []int) {
	return file_pro_client_api_proto_rawDescGZIP(), []int{0}
}

func (x *Plan) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *Plan) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *Plan) GetBestValue() bool {
	if x != nil {
		return x.BestValue
	}
	return false
}

func (x *Plan) GetUsdPrice() int64 {
	if x != nil {
		return x.UsdPrice
	}
	return 0
}

func (x *Plan) GetPrice() map[string]int64 {
	if x != nil {
		return x.Price
	}
	return nil
}

func (x *Plan) GetExpectedMonthlyPrice() map[string]int64 {
	if x != nil {
		return x.ExpectedMonthlyPrice
	}
	return nil
}

func (x *Plan) GetTotalCostBilledOneTime() string {
	if x != nil {
		return x.TotalCostBilledOneTime
	}
	return ""
}

func (x *Plan) GetOneMonthCost() string {
	if x != nil {
		return x.OneMonthCost
	}
	return ""
}

func (x *Plan) GetTotalCost() string {
	if x != nil {
		return x.TotalCost
	}
	return ""
}

func (x *Plan) GetFormattedBonus() string {
	if x != nil {
		return x.FormattedBonus
	}
	return ""
}

func (x *Plan) GetRenewalText() string {
	if x != nil {
		return x.RenewalText
	}
	return ""
}

type PlansResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Plans []*Plan `protobuf:"bytes,1,rep,name=plans,proto3" json:"plans,omitempty"`
}

func (x *PlansResponse) Reset() {
	*x = PlansResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pro_client_api_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PlansResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PlansResponse) ProtoMessage() {}

func (x *PlansResponse) ProtoReflect() protoreflect.Message {
	mi := &file_pro_client_api_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PlansResponse.ProtoReflect.Descriptor instead.
func (*PlansResponse) Descriptor() ([]byte, []int) {
	return file_pro_client_api_proto_rawDescGZIP(), []int{1}
}

func (x *PlansResponse) GetPlans() []*Plan {
	if x != nil {
		return x.Plans
	}
	return nil
}

type PaymentProvider struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name string            `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Data map[string]string `protobuf:"bytes,2,rep,name=data,proto3" json:"data,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (x *PaymentProvider) Reset() {
	*x = PaymentProvider{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pro_client_api_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PaymentProvider) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PaymentProvider) ProtoMessage() {}

func (x *PaymentProvider) ProtoReflect() protoreflect.Message {
	mi := &file_pro_client_api_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PaymentProvider.ProtoReflect.Descriptor instead.
func (*PaymentProvider) Descriptor() ([]byte, []int) {
	return file_pro_client_api_proto_rawDescGZIP(), []int{2}
}

func (x *PaymentProvider) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *PaymentProvider) GetData() map[string]string {
	if x != nil {
		return x.Data
	}
	return nil
}

type ProPaymentMethod struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Method    string             `protobuf:"bytes,1,opt,name=method,proto3" json:"method,omitempty"`
	Providers []*PaymentProvider `protobuf:"bytes,2,rep,name=providers,proto3" json:"providers,omitempty"`
}

func (x *ProPaymentMethod) Reset() {
	*x = ProPaymentMethod{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pro_client_api_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ProPaymentMethod) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ProPaymentMethod) ProtoMessage() {}

func (x *ProPaymentMethod) ProtoReflect() protoreflect.Message {
	mi := &file_pro_client_api_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ProPaymentMethod.ProtoReflect.Descriptor instead.
func (*ProPaymentMethod) Descriptor() ([]byte, []int) {
	return file_pro_client_api_proto_rawDescGZIP(), []int{3}
}

func (x *ProPaymentMethod) GetMethod() string {
	if x != nil {
		return x.Method
	}
	return ""
}

func (x *ProPaymentMethod) GetProviders() []*PaymentProvider {
	if x != nil {
		return x.Providers
	}
	return nil
}

type PaymentMethodsResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Providers map[string]*structpb.ListValue `protobuf:"bytes,1,rep,name=providers,proto3" json:"providers,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (x *PaymentMethodsResponse) Reset() {
	*x = PaymentMethodsResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pro_client_api_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PaymentMethodsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PaymentMethodsResponse) ProtoMessage() {}

func (x *PaymentMethodsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_pro_client_api_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PaymentMethodsResponse.ProtoReflect.Descriptor instead.
func (*PaymentMethodsResponse) Descriptor() ([]byte, []int) {
	return file_pro_client_api_proto_rawDescGZIP(), []int{4}
}

func (x *PaymentMethodsResponse) GetProviders() map[string]*structpb.ListValue {
	if x != nil {
		return x.Providers
	}
	return nil
}

type PurchaseRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Provider        Provider `protobuf:"varint,1,opt,name=provider,proto3,enum=Provider" json:"provider,omitempty"`
	Email           string   `protobuf:"bytes,2,opt,name=email,proto3" json:"email,omitempty"`
	Plan            string   `protobuf:"bytes,3,opt,name=plan,proto3" json:"plan,omitempty"`
	CardNumber      string   `protobuf:"bytes,4,opt,name=cardNumber,proto3" json:"cardNumber,omitempty"`
	ExpDate         string   `protobuf:"bytes,5,opt,name=expDate,proto3" json:"expDate,omitempty"`
	Cvc             string   `protobuf:"bytes,6,opt,name=cvc,proto3" json:"cvc,omitempty"`
	Currency        string   `protobuf:"bytes,7,opt,name=currency,proto3" json:"currency,omitempty"`
	DeviceName      string   `protobuf:"bytes,8,opt,name=deviceName,proto3" json:"deviceName,omitempty"`
	StripePublicKey string   `protobuf:"bytes,9,opt,name=stripePublicKey,proto3" json:"stripePublicKey,omitempty"`
	StripeEmail     string   `protobuf:"bytes,10,opt,name=stripeEmail,proto3" json:"stripeEmail,omitempty"`
	StripeToken     string   `protobuf:"bytes,11,opt,name=stripeToken,proto3" json:"stripeToken,omitempty"`
	Token           string   `protobuf:"bytes,12,opt,name=token,proto3" json:"token,omitempty"`
	ResellerCode    string   `protobuf:"bytes,13,opt,name=resellerCode,proto3" json:"resellerCode,omitempty"`
}

func (x *PurchaseRequest) Reset() {
	*x = PurchaseRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pro_client_api_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PurchaseRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PurchaseRequest) ProtoMessage() {}

func (x *PurchaseRequest) ProtoReflect() protoreflect.Message {
	mi := &file_pro_client_api_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PurchaseRequest.ProtoReflect.Descriptor instead.
func (*PurchaseRequest) Descriptor() ([]byte, []int) {
	return file_pro_client_api_proto_rawDescGZIP(), []int{5}
}

func (x *PurchaseRequest) GetProvider() Provider {
	if x != nil {
		return x.Provider
	}
	return Provider_PROVIDER_UNSET
}

func (x *PurchaseRequest) GetEmail() string {
	if x != nil {
		return x.Email
	}
	return ""
}

func (x *PurchaseRequest) GetPlan() string {
	if x != nil {
		return x.Plan
	}
	return ""
}

func (x *PurchaseRequest) GetCardNumber() string {
	if x != nil {
		return x.CardNumber
	}
	return ""
}

func (x *PurchaseRequest) GetExpDate() string {
	if x != nil {
		return x.ExpDate
	}
	return ""
}

func (x *PurchaseRequest) GetCvc() string {
	if x != nil {
		return x.Cvc
	}
	return ""
}

func (x *PurchaseRequest) GetCurrency() string {
	if x != nil {
		return x.Currency
	}
	return ""
}

func (x *PurchaseRequest) GetDeviceName() string {
	if x != nil {
		return x.DeviceName
	}
	return ""
}

func (x *PurchaseRequest) GetStripePublicKey() string {
	if x != nil {
		return x.StripePublicKey
	}
	return ""
}

func (x *PurchaseRequest) GetStripeEmail() string {
	if x != nil {
		return x.StripeEmail
	}
	return ""
}

func (x *PurchaseRequest) GetStripeToken() string {
	if x != nil {
		return x.StripeToken
	}
	return ""
}

func (x *PurchaseRequest) GetToken() string {
	if x != nil {
		return x.Token
	}
	return ""
}

func (x *PurchaseRequest) GetResellerCode() string {
	if x != nil {
		return x.ResellerCode
	}
	return ""
}

type PurchaseResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Success bool `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
}

func (x *PurchaseResponse) Reset() {
	*x = PurchaseResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pro_client_api_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PurchaseResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PurchaseResponse) ProtoMessage() {}

func (x *PurchaseResponse) ProtoReflect() protoreflect.Message {
	mi := &file_pro_client_api_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PurchaseResponse.ProtoReflect.Descriptor instead.
func (*PurchaseResponse) Descriptor() ([]byte, []int) {
	return file_pro_client_api_proto_rawDescGZIP(), []int{6}
}

func (x *PurchaseResponse) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}

var File_pro_client_api_proto protoreflect.FileDescriptor

var file_pro_client_api_proto_rawDesc = []byte{
	0x0a, 0x14, 0x70, 0x72, 0x6f, 0x2f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x2f, 0x61, 0x70, 0x69,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x73, 0x74, 0x72, 0x75, 0x63, 0x74, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x22, 0xb6, 0x04, 0x0a, 0x04, 0x50, 0x6c, 0x61, 0x6e, 0x12, 0x0e, 0x0a,
	0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x20, 0x0a,
	0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12,
	0x1c, 0x0a, 0x09, 0x62, 0x65, 0x73, 0x74, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x03, 0x20, 0x01,
	0x28, 0x08, 0x52, 0x09, 0x62, 0x65, 0x73, 0x74, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x12, 0x1a, 0x0a,
	0x08, 0x75, 0x73, 0x64, 0x50, 0x72, 0x69, 0x63, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x52,
	0x08, 0x75, 0x73, 0x64, 0x50, 0x72, 0x69, 0x63, 0x65, 0x12, 0x26, 0x0a, 0x05, 0x70, 0x72, 0x69,
	0x63, 0x65, 0x18, 0x05, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x10, 0x2e, 0x50, 0x6c, 0x61, 0x6e, 0x2e,
	0x50, 0x72, 0x69, 0x63, 0x65, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x05, 0x70, 0x72, 0x69, 0x63,
	0x65, 0x12, 0x53, 0x0a, 0x14, 0x65, 0x78, 0x70, 0x65, 0x63, 0x74, 0x65, 0x64, 0x4d, 0x6f, 0x6e,
	0x74, 0x68, 0x6c, 0x79, 0x50, 0x72, 0x69, 0x63, 0x65, 0x18, 0x06, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x1f, 0x2e, 0x50, 0x6c, 0x61, 0x6e, 0x2e, 0x45, 0x78, 0x70, 0x65, 0x63, 0x74, 0x65, 0x64, 0x4d,
	0x6f, 0x6e, 0x74, 0x68, 0x6c, 0x79, 0x50, 0x72, 0x69, 0x63, 0x65, 0x45, 0x6e, 0x74, 0x72, 0x79,
	0x52, 0x14, 0x65, 0x78, 0x70, 0x65, 0x63, 0x74, 0x65, 0x64, 0x4d, 0x6f, 0x6e, 0x74, 0x68, 0x6c,
	0x79, 0x50, 0x72, 0x69, 0x63, 0x65, 0x12, 0x36, 0x0a, 0x16, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x43,
	0x6f, 0x73, 0x74, 0x42, 0x69, 0x6c, 0x6c, 0x65, 0x64, 0x4f, 0x6e, 0x65, 0x54, 0x69, 0x6d, 0x65,
	0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x16, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x43, 0x6f, 0x73,
	0x74, 0x42, 0x69, 0x6c, 0x6c, 0x65, 0x64, 0x4f, 0x6e, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x22,
	0x0a, 0x0c, 0x6f, 0x6e, 0x65, 0x4d, 0x6f, 0x6e, 0x74, 0x68, 0x43, 0x6f, 0x73, 0x74, 0x18, 0x08,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x6f, 0x6e, 0x65, 0x4d, 0x6f, 0x6e, 0x74, 0x68, 0x43, 0x6f,
	0x73, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x43, 0x6f, 0x73, 0x74, 0x18,
	0x09, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x43, 0x6f, 0x73, 0x74,
	0x12, 0x26, 0x0a, 0x0e, 0x66, 0x6f, 0x72, 0x6d, 0x61, 0x74, 0x74, 0x65, 0x64, 0x42, 0x6f, 0x6e,
	0x75, 0x73, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x66, 0x6f, 0x72, 0x6d, 0x61, 0x74,
	0x74, 0x65, 0x64, 0x42, 0x6f, 0x6e, 0x75, 0x73, 0x12, 0x20, 0x0a, 0x0b, 0x72, 0x65, 0x6e, 0x65,
	0x77, 0x61, 0x6c, 0x54, 0x65, 0x78, 0x74, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x72,
	0x65, 0x6e, 0x65, 0x77, 0x61, 0x6c, 0x54, 0x65, 0x78, 0x74, 0x1a, 0x38, 0x0a, 0x0a, 0x50, 0x72,
	0x69, 0x63, 0x65, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61,
	0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65,
	0x3a, 0x02, 0x38, 0x01, 0x1a, 0x47, 0x0a, 0x19, 0x45, 0x78, 0x70, 0x65, 0x63, 0x74, 0x65, 0x64,
	0x4d, 0x6f, 0x6e, 0x74, 0x68, 0x6c, 0x79, 0x50, 0x72, 0x69, 0x63, 0x65, 0x45, 0x6e, 0x74, 0x72,
	0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03,
	0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x03, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22, 0x2c, 0x0a,
	0x0d, 0x50, 0x6c, 0x61, 0x6e, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x1b,
	0x0a, 0x05, 0x70, 0x6c, 0x61, 0x6e, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x05, 0x2e,
	0x50, 0x6c, 0x61, 0x6e, 0x52, 0x05, 0x70, 0x6c, 0x61, 0x6e, 0x73, 0x22, 0x8e, 0x01, 0x0a, 0x0f,
	0x50, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74, 0x50, 0x72, 0x6f, 0x76, 0x69, 0x64, 0x65, 0x72, 0x12,
	0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e,
	0x61, 0x6d, 0x65, 0x12, 0x2e, 0x0a, 0x04, 0x64, 0x61, 0x74, 0x61, 0x18, 0x02, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x1a, 0x2e, 0x50, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74, 0x50, 0x72, 0x6f, 0x76, 0x69,
	0x64, 0x65, 0x72, 0x2e, 0x44, 0x61, 0x74, 0x61, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x04, 0x64,
	0x61, 0x74, 0x61, 0x1a, 0x37, 0x0a, 0x09, 0x44, 0x61, 0x74, 0x61, 0x45, 0x6e, 0x74, 0x72, 0x79,
	0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b,
	0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22, 0x5a, 0x0a, 0x10,
	0x50, 0x72, 0x6f, 0x50, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64,
	0x12, 0x16, 0x0a, 0x06, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x06, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x12, 0x2e, 0x0a, 0x09, 0x70, 0x72, 0x6f, 0x76,
	0x69, 0x64, 0x65, 0x72, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x10, 0x2e, 0x50, 0x61,
	0x79, 0x6d, 0x65, 0x6e, 0x74, 0x50, 0x72, 0x6f, 0x76, 0x69, 0x64, 0x65, 0x72, 0x52, 0x09, 0x70,
	0x72, 0x6f, 0x76, 0x69, 0x64, 0x65, 0x72, 0x73, 0x22, 0xb8, 0x01, 0x0a, 0x16, 0x50, 0x61, 0x79,
	0x6d, 0x65, 0x6e, 0x74, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x12, 0x44, 0x0a, 0x09, 0x70, 0x72, 0x6f, 0x76, 0x69, 0x64, 0x65, 0x72, 0x73,
	0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x26, 0x2e, 0x50, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74,
	0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e,
	0x50, 0x72, 0x6f, 0x76, 0x69, 0x64, 0x65, 0x72, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x09,
	0x70, 0x72, 0x6f, 0x76, 0x69, 0x64, 0x65, 0x72, 0x73, 0x1a, 0x58, 0x0a, 0x0e, 0x50, 0x72, 0x6f,
	0x76, 0x69, 0x64, 0x65, 0x72, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b,
	0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x30, 0x0a,
	0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4c,
	0x69, 0x73, 0x74, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a,
	0x02, 0x38, 0x01, 0x22, 0x92, 0x03, 0x0a, 0x0f, 0x50, 0x75, 0x72, 0x63, 0x68, 0x61, 0x73, 0x65,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x25, 0x0a, 0x08, 0x70, 0x72, 0x6f, 0x76, 0x69,
	0x64, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x09, 0x2e, 0x50, 0x72, 0x6f, 0x76,
	0x69, 0x64, 0x65, 0x72, 0x52, 0x08, 0x70, 0x72, 0x6f, 0x76, 0x69, 0x64, 0x65, 0x72, 0x12, 0x14,
	0x0a, 0x05, 0x65, 0x6d, 0x61, 0x69, 0x6c, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x65,
	0x6d, 0x61, 0x69, 0x6c, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x6c, 0x61, 0x6e, 0x18, 0x03, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x04, 0x70, 0x6c, 0x61, 0x6e, 0x12, 0x1e, 0x0a, 0x0a, 0x63, 0x61, 0x72, 0x64,
	0x4e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x63, 0x61,
	0x72, 0x64, 0x4e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x12, 0x18, 0x0a, 0x07, 0x65, 0x78, 0x70, 0x44,
	0x61, 0x74, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x65, 0x78, 0x70, 0x44, 0x61,
	0x74, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x63, 0x76, 0x63, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x03, 0x63, 0x76, 0x63, 0x12, 0x1a, 0x0a, 0x08, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x63, 0x79,
	0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x63, 0x79,
	0x12, 0x1e, 0x0a, 0x0a, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x08,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x4e, 0x61, 0x6d, 0x65,
	0x12, 0x28, 0x0a, 0x0f, 0x73, 0x74, 0x72, 0x69, 0x70, 0x65, 0x50, 0x75, 0x62, 0x6c, 0x69, 0x63,
	0x4b, 0x65, 0x79, 0x18, 0x09, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0f, 0x73, 0x74, 0x72, 0x69, 0x70,
	0x65, 0x50, 0x75, 0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x12, 0x20, 0x0a, 0x0b, 0x73, 0x74,
	0x72, 0x69, 0x70, 0x65, 0x45, 0x6d, 0x61, 0x69, 0x6c, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0b, 0x73, 0x74, 0x72, 0x69, 0x70, 0x65, 0x45, 0x6d, 0x61, 0x69, 0x6c, 0x12, 0x20, 0x0a, 0x0b,
	0x73, 0x74, 0x72, 0x69, 0x70, 0x65, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x0b, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0b, 0x73, 0x74, 0x72, 0x69, 0x70, 0x65, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x14,
	0x0a, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x0c, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x74,
	0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x22, 0x0a, 0x0c, 0x72, 0x65, 0x73, 0x65, 0x6c, 0x6c, 0x65, 0x72,
	0x43, 0x6f, 0x64, 0x65, 0x18, 0x0d, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x72, 0x65, 0x73, 0x65,
	0x6c, 0x6c, 0x65, 0x72, 0x43, 0x6f, 0x64, 0x65, 0x22, 0x2c, 0x0a, 0x10, 0x50, 0x75, 0x72, 0x63,
	0x68, 0x61, 0x73, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x18, 0x0a, 0x07,
	0x73, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x73,
	0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x2a, 0x39, 0x0a, 0x08, 0x50, 0x72, 0x6f, 0x76, 0x69, 0x64,
	0x65, 0x72, 0x12, 0x12, 0x0a, 0x0e, 0x50, 0x52, 0x4f, 0x56, 0x49, 0x44, 0x45, 0x52, 0x5f, 0x55,
	0x4e, 0x53, 0x45, 0x54, 0x10, 0x00, 0x12, 0x0a, 0x0a, 0x06, 0x53, 0x54, 0x52, 0x49, 0x50, 0x45,
	0x10, 0x01, 0x12, 0x0d, 0x0a, 0x09, 0x46, 0x52, 0x45, 0x45, 0x4b, 0x41, 0x53, 0x53, 0x41, 0x10,
	0x02, 0x42, 0x2d, 0x5a, 0x2b, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f,
	0x67, 0x65, 0x74, 0x6c, 0x61, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x2f, 0x66, 0x6c, 0x61, 0x73, 0x68,
	0x6c, 0x69, 0x67, 0x68, 0x74, 0x2f, 0x70, 0x72, 0x6f, 0x2f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_pro_client_api_proto_rawDescOnce sync.Once
	file_pro_client_api_proto_rawDescData = file_pro_client_api_proto_rawDesc
)

func file_pro_client_api_proto_rawDescGZIP() []byte {
	file_pro_client_api_proto_rawDescOnce.Do(func() {
		file_pro_client_api_proto_rawDescData = protoimpl.X.CompressGZIP(file_pro_client_api_proto_rawDescData)
	})
	return file_pro_client_api_proto_rawDescData
}

var file_pro_client_api_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_pro_client_api_proto_msgTypes = make([]protoimpl.MessageInfo, 11)
var file_pro_client_api_proto_goTypes = []interface{}{
	(Provider)(0),                  // 0: Provider
	(*Plan)(nil),                   // 1: Plan
	(*PlansResponse)(nil),          // 2: PlansResponse
	(*PaymentProvider)(nil),        // 3: PaymentProvider
	(*ProPaymentMethod)(nil),       // 4: ProPaymentMethod
	(*PaymentMethodsResponse)(nil), // 5: PaymentMethodsResponse
	(*PurchaseRequest)(nil),        // 6: PurchaseRequest
	(*PurchaseResponse)(nil),       // 7: PurchaseResponse
	nil,                            // 8: Plan.PriceEntry
	nil,                            // 9: Plan.ExpectedMonthlyPriceEntry
	nil,                            // 10: PaymentProvider.DataEntry
	nil,                            // 11: PaymentMethodsResponse.ProvidersEntry
	(*structpb.ListValue)(nil),     // 12: google.protobuf.ListValue
}
var file_pro_client_api_proto_depIdxs = []int32{
	8,  // 0: Plan.price:type_name -> Plan.PriceEntry
	9,  // 1: Plan.expectedMonthlyPrice:type_name -> Plan.ExpectedMonthlyPriceEntry
	1,  // 2: PlansResponse.plans:type_name -> Plan
	10, // 3: PaymentProvider.data:type_name -> PaymentProvider.DataEntry
	3,  // 4: ProPaymentMethod.providers:type_name -> PaymentProvider
	11, // 5: PaymentMethodsResponse.providers:type_name -> PaymentMethodsResponse.ProvidersEntry
	0,  // 6: PurchaseRequest.provider:type_name -> Provider
	12, // 7: PaymentMethodsResponse.ProvidersEntry.value:type_name -> google.protobuf.ListValue
	8,  // [8:8] is the sub-list for method output_type
	8,  // [8:8] is the sub-list for method input_type
	8,  // [8:8] is the sub-list for extension type_name
	8,  // [8:8] is the sub-list for extension extendee
	0,  // [0:8] is the sub-list for field type_name
}

func init() { file_pro_client_api_proto_init() }
func file_pro_client_api_proto_init() {
	if File_pro_client_api_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_pro_client_api_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Plan); i {
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
		file_pro_client_api_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PlansResponse); i {
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
		file_pro_client_api_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PaymentProvider); i {
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
		file_pro_client_api_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ProPaymentMethod); i {
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
		file_pro_client_api_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PaymentMethodsResponse); i {
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
		file_pro_client_api_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PurchaseRequest); i {
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
		file_pro_client_api_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PurchaseResponse); i {
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
			RawDescriptor: file_pro_client_api_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   11,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_pro_client_api_proto_goTypes,
		DependencyIndexes: file_pro_client_api_proto_depIdxs,
		EnumInfos:         file_pro_client_api_proto_enumTypes,
		MessageInfos:      file_pro_client_api_proto_msgTypes,
	}.Build()
	File_pro_client_api_proto = out.File
	file_pro_client_api_proto_rawDesc = nil
	file_pro_client_api_proto_goTypes = nil
	file_pro_client_api_proto_depIdxs = nil
}
