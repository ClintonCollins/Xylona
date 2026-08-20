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
