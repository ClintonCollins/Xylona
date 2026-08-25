# Xylona

Xylona manages the configuration and runtime lifecycle of game servers.

## Language

**Start arguments**:
The ordered, platform-specific command-line values used to launch a game server. A game definition supplies the baseline, and a game server may store permitted customizations.
_Avoid_: Startup flags, launch parameters

**Start-argument patch**:
A stored game-server customization that adds, edits, or removes part of the start arguments. A patch whose referenced template entry no longer exists has no effect.
_Avoid_: Override

**Start-argument blocklist**:
A game-level set of forbidden token patterns applied to the effective start arguments before they are saved or launched.
_Avoid_: Denylist

**Game server owner**:
The single User with full authority over a game server. Ownership is distinct from access granted to other Users through roles.

**Player**:
A participant reported by a game server or its query interface. Groups and lists of players are still called players.
_Avoid_: Roster

**Game server status page**:
An owner-controlled public view of the live status, players, and connection addresses for every game server they own.

**Game server map share**:
An owner-controlled public identifier that exposes the live map of one supported game server while both the share and any required map configuration are enabled.
_Avoid_: Public map token, shared map token
