package push

import (
	"net/http"

	"github.com/denverdino/aliyungo/common"
)

const (
	PushTargetDevice  = "DEVICE"
	PushTargetAccount = "ACCOUNT"
	PushTargetAlias   = "ALIAS"
	PushTargetTag     = "TAG"
	PushTargetAll     = "ALL"

	PushDeviceTypeIOS     = "iOS"
	PushDeviceTypeAndroid = "ANDROID"
	PushDeviceTypeAll     = "ALL"

	PushTypeMessage = "MESSAGE"
	PushTypeNotice  = "NOTICE"

	PushIOSAPNENVProduct     = "PRODUCT"
	PushIOSAPNENVDevelopment = "DEV"
)

//高级推送参数
type PushArgs struct {
	/*----基础参数----*/
	//AppKey信息
	AppKey int64
	/*----推送目标----*/
	//推送目标
	Target string
	//根据Target来设定，多个值使用逗号分隔，最多支持100个。
	TargetValue string
	//设备类型
	DeviceType string
	/*----推送配置----*/
	PushType string
	//Android消息标题,Android通知标题,iOS消息标题
	Title string
	//Android消息内容,Android通知内容,iOS消息内容
	Body string
	//[iOS通知内容]
	Summary string
	/*----下述配置仅作用于iOS通知任务----*/
	//[iOS通知声音]
	IOSMusic string `ArgName:"iOSMusic"`
	//[iOS应用图标右上角角标]
	IOSBadge int `ArgName:"iOSBadge"`
	//[iOS通知标题（iOS 10+通知显示标题]
	IOSTitle string `ArgName:"iOSTitle"`
	//[开启iOS静默通知]
	IOSSilentNotification string `ArgName:"iOSSilentNotification"`
	//[iOS通知副标题（iOS 10+）]
	IOSSubtitle string `ArgName:"iOSSubtitle"`
	//[设定iOS通知Category（iOS 10+）]
	IOSNotificationCategory string `ArgName:"iOSNotificationCategory"`
	//[是否使能iOS通知扩展处理（iOS 10+）]
	IOSMutableContent string `ArgName:"iOSMutableContent"`
	//[iOS通知的扩展属性]
	IOSExtParameters string `ArgName:"iOSExtParameters"`
	//[环境信息]
	IOSApnsEnv string `ArgName:"iOSApnsEnv"`
	//[推送时设备不在线则这条推送会做为通知]
	IOSRemind bool `ArgName:"iOSRemind"`
	//[iOS消息转通知时使用的iOS通知内容]
	IOSRemindBody string `ArgName:"iOSRemindBody"`
	/*----下述配置仅作用于Android通知任务----*/
	//[Android通知声音]
	AndroidMusic string
	//[点击通知后动作]
	AndroidOpenType string
	//通知的提醒方式
	AndroidNotifyType string
	//[设定通知打开的activity]
	AndroidActivity string
	//[Android收到推送后打开对应的url]
	AndroidOpenUrl string
	//[Android自定义通知栏样式]
	AndroidNotificationBarType int
	//[Android通知在通知栏展示时排列位置的优先级]
	AndroidNotificationBarPriority int
	//[Android NotificationChannel 参数，兼容 8.0 系统]
	AndroidNotificationChannel string
	//[设定通知的扩展属性]
	AndroidExtParameters string
	/*----下述配置仅作用于Android辅助弹窗功能----*/
	//[推送类型为消息时设备不在线，则这条推送会使用辅助弹窗功能]
	AndroidRemind bool
	//[此处指定通知点击后跳转的Activity]
	AndroidPopupActivity string
	//[辅助弹窗模式下Title内容,长度限制:<16字符（中英文都以一个字符计算）]
	AndroidPopupTitle string
	//[辅助弹窗模式下Body内容,长度限制:<128字符（中英文都以一个字符计算）]
	AndroidPopupBody string
	/*----推送控制----*/
	//[用于定时发送]
	PushTime string
	//[离线消息/通知是否保存]
	StoreOffline string
	//[离线消息/通知的过期时间]
	ExpireTime string
	/*----短信融合----*/
	//补发短信的模板名
	SmsTemplateName string
	//补发短信的签名
	SmsSignName string
	//短信模板的变量名值对
	SmsParams string
	//触发短信的延迟时间，秒
	SmsDelaySecs int
	//触发短信的条件
	SmsSendPolicy int
}

type PushResponse struct {
	common.Response
	MessageId string
}

func (this *Client) Push(args *PushArgs) (*PushResponse, error) {
	resp := PushResponse{}
	return &resp, this.InvokeByAnyMethod(http.MethodPost, Push, "", args, &resp)
}
