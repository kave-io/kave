package casbin

import (
	"context"
	"fmt"
)

// basePermissions defines static role -> domain -> resource -> action grants.
// Policies are persisted through Casbin adapter (`casbin_rule`) and are idempotent.
var basePermissions = []PermissionPolicy{
	// Super admin: full access (even if bypass is disabled).
	{RolePlatformSuperAdmin, WildcardDomain, WildcardResource, WildcardAction, EffectAllow},

	// Platform admin (system scope).
	{RolePlatformAdmin, DomainSys, ResourceSystem, WildcardAction, EffectAllow},
	{RolePlatformAdmin, DomainSys, ResourceRBAC, WildcardAction, EffectAllow},
	{RolePlatformAdmin, DomainSys, ResourceAudit, ActionRead, EffectAllow},

	// Platform moderator (cross-domain moderation/content controls).
	{RolePlatformModerator, WildcardDomain, ResourceModAction, WildcardAction, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceModAppeal, WildcardAction, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceModBlocklist, WildcardAction, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceModContentFlag, WildcardAction, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceModReport, WildcardAction, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceModReportReason, WildcardAction, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceModShadowBan, WildcardAction, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceModUserBan, WildcardAction, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceContentPost, ActionModerate, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceSocialComment, ActionModerate, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceIdentityUser, ActionSuspend, EffectAllow},
	{RolePlatformModerator, WildcardDomain, ResourceIdentityUser, ActionUnsuspend, EffectAllow},

	// User self (usually assigned in user:<id>).
	{RoleUserSelf, WildcardDomain, ResourceIdentityUser, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceIdentityProfile, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceIdentityDevice, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceIdentityBlock, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceIdentityAuthSession, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceIdentityRefreshToken, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceIdentityOTP, ActionManage, EffectAllow},

	{RoleUserSelf, WildcardDomain, ResourceSocialFollow, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceSocialLike, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceSocialSave, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceSocialComment, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceSocialNotification, ActionManage, EffectAllow},

	{RoleUserSelf, WildcardDomain, ResourceGeoLocation, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceUserLocation, ActionManage, EffectAllow},

	{RoleUserSelf, WildcardDomain, ResourceDiscoveryQuestion, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryDimension, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryBoundaryOption, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryCategory, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryTag, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryStatement, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryQuestionResponse, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoverySignal, ActionCreate, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryUserDimensionState, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryUserQuestionState, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryUserBoundary, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryQuestionRecommendation, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryContentRecommendation, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDiscoveryProfileRecommendation, ActionRead, EffectAllow},

	{RoleUserSelf, WildcardDomain, ResourceDatingPreference, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDatingLocation, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDatingInteraction, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceDatingMatch, ActionRead, EffectAllow},

	{RoleUserSelf, WildcardDomain, ResourceMsgConversation, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceMsgParticipant, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceMsgMessage, ActionSend, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceMsgMessage, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceMsgMessageReaction, ActionReact, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceMsgMessageReceipt, ActionManage, EffectAllow},

	{RoleUserSelf, WildcardDomain, ResourceContentPost, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceContentMedia, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceContentTag, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceContentCollection, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceContentPurchase, ActionRead, EffectAllow},

	{RoleUserSelf, WildcardDomain, ResourceMoneyWallet, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceMoneyTip, ActionManage, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceMoneyLedgerEntry, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceMoneyPayout, ActionRead, EffectAllow},
	{RoleUserSelf, WildcardDomain, ResourceMoneyRefund, ActionManage, EffectAllow},

	// Creator self (usually assigned in creator:<id>).
	{RoleCreatorSelf, WildcardDomain, ResourceCreatorProfile, ActionManage, EffectAllow},
	{RoleCreatorSelf, WildcardDomain, ResourceCreatorCategory, ActionManage, EffectAllow},
	{RoleCreatorSelf, WildcardDomain, ResourceCreatorEvent, ActionManage, EffectAllow},
	{RoleCreatorSelf, WildcardDomain, ResourceCreatorKYC, ActionManage, EffectAllow},
	{RoleCreatorSelf, WildcardDomain, ResourceCreatorVerification, ActionRead, EffectAllow},
	{RoleCreatorSelf, WildcardDomain, ResourceCreatorPayoutAccount, ActionManage, EffectAllow},
	{RoleCreatorSelf, WildcardDomain, ResourceMoneyPayout, ActionManage, EffectAllow},
	{RoleCreatorSelf, WildcardDomain, ResourceMoneyWallet, ActionRead, EffectAllow},
	{RoleCreatorSelf, WildcardDomain, ResourceMoneyLedgerEntry, ActionRead, EffectAllow},

	// Conversation participant (usually assigned in conversation:<id>).
	{RoleConversationParticipant, WildcardDomain, ResourceMsgConversation, ActionRead, EffectAllow},
	{RoleConversationParticipant, WildcardDomain, ResourceMsgParticipant, ActionRead, EffectAllow},
	{RoleConversationParticipant, WildcardDomain, ResourceMsgMessage, ActionSend, EffectAllow},
	{RoleConversationParticipant, WildcardDomain, ResourceMsgMessage, ActionRead, EffectAllow},
	{RoleConversationParticipant, WildcardDomain, ResourceMsgMessageReaction, ActionReact, EffectAllow},
	{RoleConversationParticipant, WildcardDomain, ResourceMsgMessageReceipt, ActionManage, EffectAllow},

	// Internal services / workers.
	{RoleServiceInternal, WildcardDomain, WildcardResource, WildcardAction, EffectAllow},
}

// SeedBasePermissions loads static role permissions into the enforcer.
func SeedBasePermissions(ctx context.Context, c Casbin) error {
	for _, p := range basePermissions {
		if _, err := c.AddPermission(ctx, p.Subject, p.Domain, p.Object, p.Action, p.Effect); err != nil {
			return fmt.Errorf("casbin: seed %v: %w", p, err)
		}
	}
	return nil
}
