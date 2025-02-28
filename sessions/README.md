# Steam Session Management

This directory stores Steam session data, including refresh tokens, for the CS2 Inspect Service bots.

## Session Format

Each session is stored as a JSON file with the following format:

```json
{
	"refreshToken": "eyAidHlwIjogIkpXVCIsICJhbGciOiAiRWREU0EiIH0...",
	"timestamp": 1733498317715,
	"username": "bot_username",
	"hasGuard": false
}
```

## How Refresh Tokens Work

1. When a bot successfully logs in, Steam may provide a refresh token
2. The refresh token is saved to a session file in this directory
3. On subsequent startups, the bot will attempt to use the refresh token instead of password authentication
4. If the refresh token is valid, the bot can log in without needing the password
5. If the refresh token is invalid or expired, the bot will fall back to password authentication
6. Refresh tokens are valid for approximately 90 days, but we consider them expired after 180 days as a safety measure

## Benefits of Using Refresh Tokens

- Reduced need for password authentication
- Better handling of Steam Guard requirements
- More reliable reconnection after service restarts
- Improved security by reducing the frequency of password transmission

## Implementation Details

The refresh token handling is implemented in the following files:

- `bot.go`: Contains the `loadSession` and `saveSession` methods
- `models.go`: Defines the `Session` struct
- `accounts.txt`: Contains the bot credentials (username, password, etc.)

The session files are named after the bot's username (e.g., `bot_username.json`).
