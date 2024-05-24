# Internal Calls
This is used to write custom functions to handle calls, such as Games. Instead of using direct, bash, or powershell,
you can write Go code to handle certain commands.

### Games
Games houses all of our built-in games. You can add your own games anywhere as long as they implement the `Game` interface.
Once you have a `Game` interface implemented, you can call `internal.RegisterGame` to add it to the list of games.
Then, you can add the game in Xylona and use internal for any of the commands you'd like, `install, update`.