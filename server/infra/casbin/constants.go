package casbin

import "fmt"

type Action string
type Resource string
type Role string
type Domain string

// ----------------------------
// Actions
// ----------------------------

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionList   Action = "list"

	// Power action: expands to CRUD + list in model matcher.
	ActionManage Action = "manage"

	// Lifecycle / moderation style actions.
	ActionPublish   Action = "publish"
	ActionArchive   Action = "archive"
	ActionClose     Action = "close"
	ActionSuspend   Action = "suspend"
	ActionUnsuspend Action = "unsuspend"
	ActionBlock     Action = "block"
	ActionUnblock   Action = "unblock"
	ActionModerate  Action = "moderate"

	// Messaging / social style actions.
	ActionSend  Action = "send"
	ActionReact Action = "react"

	// RBAC-specific actions.
	ActionGrant  Action = "grant"
	ActionRevoke Action = "revoke"
)

const (
	WildcardAction Action = "*"
)

var KnownActions = map[Action]struct{}{
	ActionCreate: {}, ActionRead: {}, ActionUpdate: {}, ActionDelete: {}, ActionList: {}, ActionManage: {},
	ActionPublish: {}, ActionArchive: {}, ActionClose: {},
	ActionSuspend: {}, ActionUnsuspend: {}, ActionBlock: {}, ActionUnblock: {}, ActionModerate: {},
	ActionSend: {}, ActionReact: {},
	ActionGrant: {}, ActionRevoke: {},
}

// ----------------------------
// Resources
// ----------------------------

const (
	WildcardResource Resource = "*"

	// System.
	ResourceSystem Resource = "system"
	ResourceAudit  Resource = "audit"
	ResourceRBAC   Resource = "rbac"

	// Identity / auth.
	ResourceIdentityUser         Resource = "identity/user"
	ResourceIdentityProfile      Resource = "identity/profile"
	ResourceIdentityDevice       Resource = "identity/device"
	ResourceIdentityBlock        Resource = "identity/block"
	ResourceIdentityAuthSession  Resource = "identity/auth_session"
	ResourceIdentityRefreshToken Resource = "identity/refresh_token"
	ResourceIdentityOTP          Resource = "identity/otp"

	// Social.
	ResourceSocialFollow       Resource = "social/follow"
	ResourceSocialLike         Resource = "social/like"
	ResourceSocialSave         Resource = "social/save"
	ResourceSocialComment      Resource = "social/comment"
	ResourceSocialNotification Resource = "social/notification"

	// Geo.
	ResourceGeoLocation  Resource = "geo/location"
	ResourceUserLocation Resource = "geo/user_location"

	// Discovery.
	ResourceDiscoveryDimension              Resource = "discovery/dimension"
	ResourceDiscoveryQuestion               Resource = "discovery/question"
	ResourceDiscoveryQuestionMapping        Resource = "discovery/question_mapping"
	ResourceDiscoveryBoundaryOption         Resource = "discovery/boundary_option"
	ResourceDiscoverySignal                 Resource = "discovery/signal"
	ResourceDiscoveryQuestionResponse       Resource = "discovery/question_response"
	ResourceDiscoveryUserDimensionState     Resource = "discovery/user_dimension_state"
	ResourceDiscoveryUserQuestionState      Resource = "discovery/user_question_state"
	ResourceDiscoveryUserBoundary           Resource = "discovery/user_boundary"
	ResourceDiscoveryCategory               Resource = "discovery/category"
	ResourceDiscoveryTag                    Resource = "discovery/tag"
	ResourceDiscoveryStatement              Resource = "discovery/statement"
	ResourceDiscoveryQuestionRecommendation Resource = "discovery/question_recommendation"
	ResourceDiscoveryContentRecommendation  Resource = "discovery/content_recommendation"
	ResourceDiscoveryProfileRecommendation  Resource = "discovery/profile_recommendation"

	// Dating.
	ResourceDatingPreference  Resource = "dating/preference"
	ResourceDatingLocation    Resource = "dating/location"
	ResourceDatingInteraction Resource = "dating/interaction"
	ResourceDatingMatch       Resource = "dating/match"

	// Messaging.
	ResourceMsgConversation    Resource = "msg/conversation"
	ResourceMsgParticipant     Resource = "msg/participant"
	ResourceMsgMessage         Resource = "msg/message"
	ResourceMsgMessageReceipt  Resource = "msg/message_receipt"
	ResourceMsgMessageReaction Resource = "msg/message_reaction"

	// Content.
	ResourceContentPost       Resource = "content/post"
	ResourceContentMedia      Resource = "content/media"
	ResourceContentTag        Resource = "content/tag"
	ResourceContentCollection Resource = "content/collection"
	ResourceContentPurchase   Resource = "content/purchase"

	// Creator.
	ResourceCreatorProfile       Resource = "creator/profile"
	ResourceCreatorCategory      Resource = "creator/category"
	ResourceCreatorEvent         Resource = "creator/event"
	ResourceCreatorKYC           Resource = "creator/kyc"
	ResourceCreatorVerification  Resource = "creator/verification"
	ResourceCreatorPayoutAccount Resource = "creator/payout_account"

	// Money.
	ResourceMoneyWallet      Resource = "money/wallet"
	ResourceMoneyLedgerEntry Resource = "money/ledger_entry"
	ResourceMoneyTip         Resource = "money/tip"
	ResourceMoneyPayout      Resource = "money/payout"
	ResourceMoneyPlatformFee Resource = "money/platform_fee"
	ResourceMoneyRefund      Resource = "money/refund"

	// Moderation.
	ResourceModAction       Resource = "mod/action"
	ResourceModAppeal       Resource = "mod/appeal"
	ResourceModBlocklist    Resource = "mod/blocklist"
	ResourceModContentFlag  Resource = "mod/content_flag"
	ResourceModReport       Resource = "mod/report"
	ResourceModReportReason Resource = "mod/report_reason"
	ResourceModShadowBan    Resource = "mod/shadow_ban"
	ResourceModUserBan      Resource = "mod/user_ban"
)

var KnownResources = map[Resource]struct{}{
	ResourceSystem: {}, ResourceAudit: {}, ResourceRBAC: {},
	ResourceIdentityUser: {}, ResourceIdentityProfile: {}, ResourceIdentityDevice: {}, ResourceIdentityBlock: {},
	ResourceIdentityAuthSession: {}, ResourceIdentityRefreshToken: {}, ResourceIdentityOTP: {},
	ResourceSocialFollow: {}, ResourceSocialLike: {}, ResourceSocialSave: {}, ResourceSocialComment: {}, ResourceSocialNotification: {},
	ResourceGeoLocation: {}, ResourceUserLocation: {},
	ResourceDiscoveryDimension: {}, ResourceDiscoveryQuestion: {}, ResourceDiscoveryQuestionMapping: {}, ResourceDiscoveryBoundaryOption: {},
	ResourceDiscoverySignal: {}, ResourceDiscoveryQuestionResponse: {}, ResourceDiscoveryUserDimensionState: {},
	ResourceDiscoveryUserQuestionState: {}, ResourceDiscoveryUserBoundary: {}, ResourceDiscoveryCategory: {}, ResourceDiscoveryTag: {},
	ResourceDiscoveryStatement: {}, ResourceDiscoveryQuestionRecommendation: {}, ResourceDiscoveryContentRecommendation: {},
	ResourceDiscoveryProfileRecommendation: {},
	ResourceDatingPreference:               {}, ResourceDatingLocation: {}, ResourceDatingInteraction: {}, ResourceDatingMatch: {},
	ResourceMsgConversation: {}, ResourceMsgParticipant: {}, ResourceMsgMessage: {}, ResourceMsgMessageReceipt: {}, ResourceMsgMessageReaction: {},
	ResourceContentPost: {}, ResourceContentMedia: {}, ResourceContentTag: {}, ResourceContentCollection: {}, ResourceContentPurchase: {},
	ResourceCreatorProfile: {}, ResourceCreatorCategory: {}, ResourceCreatorEvent: {}, ResourceCreatorKYC: {},
	ResourceCreatorVerification: {}, ResourceCreatorPayoutAccount: {},
	ResourceMoneyWallet: {}, ResourceMoneyLedgerEntry: {}, ResourceMoneyTip: {}, ResourceMoneyPayout: {},
	ResourceMoneyPlatformFee: {}, ResourceMoneyRefund: {},
	ResourceModAction: {}, ResourceModAppeal: {}, ResourceModBlocklist: {}, ResourceModContentFlag: {},
	ResourceModReport: {}, ResourceModReportReason: {}, ResourceModShadowBan: {}, ResourceModUserBan: {},
}

// ----------------------------
// Roles
// ----------------------------
//
// Roles are policy subjects (p.sub) and assigned to principals through g.

const (
	WildcardRole Role = "*"

	RolePlatformSuperAdmin Role = "role:platform:superadmin"
	RolePlatformAdmin      Role = "role:platform:admin"
	RolePlatformModerator  Role = "role:platform:moderator"

	RoleUserSelf                Role = "role:user:self"
	RoleCreatorSelf             Role = "role:creator:self"
	RoleConversationParticipant Role = "role:conversation:participant"

	RoleServiceInternal Role = "role:service:internal"
)

var KnownRoles = map[Role]struct{}{
	RolePlatformSuperAdmin: {}, RolePlatformAdmin: {}, RolePlatformModerator: {},
	RoleUserSelf: {}, RoleCreatorSelf: {}, RoleConversationParticipant: {},
	RoleServiceInternal: {},
}

var RoleDisplayNamesFA = map[Role]string{
	RolePlatformSuperAdmin:      "سوپرادمین پلتفرم",
	RolePlatformAdmin:           "ادمین پلتفرم",
	RolePlatformModerator:       "ناظر پلتفرم",
	RoleUserSelf:                "مالک حساب",
	RoleCreatorSelf:             "مالک کریتور",
	RoleConversationParticipant: "عضو گفتگو",
	RoleServiceInternal:         "سرویس داخلی",
}

// ----------------------------
// Domains
// ----------------------------

const (
	DomainSys Domain = "sys"
)

const (
	DomainPrefixUser         Domain = "user:"
	DomainPrefixCreator      Domain = "creator:"
	DomainPrefixConversation Domain = "conversation:"
	DomainPrefixMatch        Domain = "match:"
	DomainPrefixPost         Domain = "post:"
)

const (
	WildcardDomain Domain = "*"
)

func UserDomain(userID string) Domain {
	return Domain(fmt.Sprintf("%s%s", DomainPrefixUser, userID))
}

func CreatorDomain(creatorID string) Domain {
	return Domain(fmt.Sprintf("%s%s", DomainPrefixCreator, creatorID))
}

func ConversationDomain(conversationID string) Domain {
	return Domain(fmt.Sprintf("%s%s", DomainPrefixConversation, conversationID))
}

func MatchDomain(matchID string) Domain {
	return Domain(fmt.Sprintf("%s%s", DomainPrefixMatch, matchID))
}

func PostDomain(postID string) Domain {
	return Domain(fmt.Sprintf("%s%s", DomainPrefixPost, postID))
}

// ----------------------------
// Casbin tuple helpers
// ----------------------------

type PolicyEffect string

const (
	EffectAllow PolicyEffect = "allow"
	EffectDeny  PolicyEffect = "deny"
)

// GroupSubject is the g.sub in Casbin: a concrete principal id.
type GroupSubject string

// Grouping rows: g, principal_id, role, domain
type GroupingPolicy struct {
	Subject GroupSubject
	Role    Role
	Domain  Domain
}

// Permission rows: p, role, domain, resource, action, eft
type PermissionPolicy struct {
	Subject Role
	Domain  Domain
	Object  Resource
	Action  Action
	Effect  PolicyEffect
}
