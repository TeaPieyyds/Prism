package protocol

import (
	"image/color"

	"github.com/google/uuid"
)

const (
	PlayerMovementModeClient = iota
	PlayerMovementModeServer
	PlayerMovementModeServerWithRewind
)

const (
	// Netease/PhoenixBuilder compatibility placeholder kept for legacy packet handling.
	PlayerActionClientAckServerData = 36
)

const (
	PlayerActionStartBreak = iota
	PlayerActionAbortBreak
	PlayerActionStopBreak
	PlayerActionGetUpdatedBlock
	PlayerActionDropItem
	PlayerActionStartSleeping
	PlayerActionStopSleeping
	PlayerActionRespawn
	PlayerActionJump
	PlayerActionStartSprint
	PlayerActionStopSprint
	PlayerActionStartSneak
	PlayerActionStopSneak
	PlayerActionCreativePlayerDestroyBlock
	PlayerActionDimensionChangeDone
	PlayerActionStartGlide
	PlayerActionStopGlide
	PlayerActionBuildDenied
	PlayerActionCrackBreak
	PlayerActionChangeSkin
	PlayerActionSetEnchantmentSeed
	PlayerActionStartSwimming
	PlayerActionStopSwimming
	PlayerActionStartSpinAttack
	PlayerActionStopSpinAttack
	PlayerActionStartBuildingBlock
	PlayerActionPredictDestroyBlock
	PlayerActionContinueDestroyBlock
	PlayerActionStartItemUseOn
	PlayerActionStopItemUseOn
	PlayerActionHandledTeleport
	PlayerActionMissedSwing
	PlayerActionStartCrawling
	PlayerActionStopCrawling
	PlayerActionStartFlying
	PlayerActionStopFlying
	_
	PlayerActionStartUsingItem
)

// PlayerListEntry is an entry found in the PlayerList packet. It represents a single player using the UUID
// found in the entry, and contains several properties such as the skin.
type PlayerListEntry struct {
	// UUID is the UUID of the player as sent in the Login packet when the client joined the server. It must
	// match this UUID exactly for the correct XBOX Live icon to show up in the list.
	UUID uuid.UUID
	// EntityUniqueID is the unique entity ID of the player. This ID typically stays consistent during the
	// lifetime of a world, but servers often send the runtime ID for this.
	EntityUniqueID int64
	// Username is the username that is shown in the player list of the player that obtains a PlayerList
	// packet with this entry. It does not have to be the same as the actual username of the player.
	Username string
	// XUID is the XBOX Live user ID of the player, which will remain consistent as long as the player is
	// logged in with the XBOX Live account.
	XUID string
	// PlatformChatID is an identifier only set for particular platforms when chatting (presumably only for
	// Nintendo Switch). It is otherwise an empty string, and is used to decide which players are able to
	// chat with each other.
	PlatformChatID string
	// BuildPlatform is the platform of the player as sent by that player in the Login packet.
	BuildPlatform int32
	// Skin is the skin of the player that should be added to the player list. Once sent here, it will not
	// have to be sent again.
	Skin Skin
	// Teacher is a Minecraft: Education Edition field. It specifies if the player to be added to the player
	// list is a teacher.
	Teacher bool
	// Host specifies if the player that is added to the player list is the host of the game.
	Host bool
	// SubClient specifies if the player that is added to the player list is a sub-client of another player.
	SubClient bool
	// PlayerColour is the colour of the player that is shown in UI elements, currently only used for the
	// player locator bar.
	PlayerColour color.RGBA
}

// Marshal encodes/decodes a PlayerListEntry.
func (x *PlayerListEntry) Marshal(r IO) {
	r.UUID(&x.UUID)
	r.Varint64(&x.EntityUniqueID)
	r.String(&x.Username)
	r.String(&x.XUID)
	r.String(&x.PlatformChatID)
	r.Int32(&x.BuildPlatform)
	Single(r, &x.Skin)
	r.Bool(&x.Teacher)
	r.Bool(&x.Host)
	r.Bool(&x.SubClient)
	r.ARGB(&x.PlayerColour)
}

// NetEaseUnknownPlayerListEntry ...
type NeteaseUnknownPlayerListEntry struct {
	// Netease: whether the entry has a value or not.
	HasValue bool
	// Netease
	Unknown1 bool
	// Netease
	Unknown2 string
	// Netease
	Unknown3 string
	// Netease
	Unknown4 string
	// Netease
	Unknown5 string
	// Netease
	Unknown6 string
}

// Marshal encodes/decodes a NeteaseUnknownPlayerListEntry.
func (x *NeteaseUnknownPlayerListEntry) Marshal(r IO) {
	if r.Bool(&x.HasValue); x.HasValue {
		r.Bool(&x.Unknown1)
		r.String(&x.Unknown2)
		r.String(&x.Unknown3)
		r.String(&x.Unknown4)
		r.String(&x.Unknown5)
		r.String(&x.Unknown6)
	}
}

// PlayerListRemoveEntry encodes/decodes a PlayerListEntry for removal from the list.
func PlayerListRemoveEntry(r IO, x *PlayerListEntry) {
	r.UUID(&x.UUID)
}

// PlayerMovementSettings represents the different server authoritative movement settings. These control how
// the client will provide input to the server.
type PlayerMovementSettings struct {
	// MovementType is kept in-memory for legacy compatibility, but is no longer encoded in 1.21.90.
	MovementType int32
	// RewindHistorySize is the amount of history to keep at maximum.
	RewindHistorySize int32
	// ServerAuthoritativeBlockBreaking specifies if block breaking should be sent through
	// packet.PlayerAuthInput or not.
	ServerAuthoritativeBlockBreaking bool
}

// PlayerMoveSettings reads/writes PlayerMovementSettings x to/from IO r.
func PlayerMoveSettings(r IO, x *PlayerMovementSettings) {
	r.Varint32(&x.RewindHistorySize)
	r.Bool(&x.ServerAuthoritativeBlockBreaking)
}

// PlayerBlockAction ...
type PlayerBlockAction struct {
	// Action is the action to be performed, and is one of the constants listed above.
	Action int32
	// BlockPos is the position of the block that was interacted with.
	BlockPos BlockPos
	// Face is the face of the block that was interacted with.
	Face int32
}

// Marshal encodes/decodes a PlayerBlockAction.
func (x *PlayerBlockAction) Marshal(r IO) {
	r.Varint32(&x.Action)
	switch x.Action {
	case PlayerActionStartBreak, PlayerActionAbortBreak, PlayerActionCrackBreak, PlayerActionPredictDestroyBlock, PlayerActionContinueDestroyBlock:
		r.BlockPos(&x.BlockPos)
		r.Varint32(&x.Face)
	}
}

// NeteasePlayerData ...
type NeteasePlayerData struct {
	// Netease
	Unknown1 bool
	// Netease
	UUID uuid.UUID
	// Netease
	Unknown2 string
}

// Marshal ...
func (x *NeteasePlayerData) Marshal(r IO) {
	r.Bool(&x.Unknown1)
	r.UUID(&x.UUID)
	r.String(&x.Unknown2)
}

// PlayerArmourDamageEntry represents an entry for a single piece of armour that should be damaged.
type PlayerArmourDamageEntry struct {
	// ArmourSlot is the index of the armour slot to damage.
	ArmourSlot int32
	// Damage is the amount of damage to apply to the armour in the specified slot.
	Damage int16
}

// Marshal encodes/decodes a PlayerArmourDamageEntry.
func (x *PlayerArmourDamageEntry) Marshal(r IO) {
	r.Varint32(&x.ArmourSlot)
	r.Int16(&x.Damage)
}
