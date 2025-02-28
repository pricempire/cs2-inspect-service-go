# Account Blacklist System

The CS2 Inspect Service includes an account blacklist system to handle Steam accounts that have been banned, have Steam Guard issues, or have invalid credentials. This document explains how the blacklist system works and how to manage it.

## Overview

The blacklist system automatically detects and records accounts that encounter specific login issues, preventing the service from repeatedly trying to use accounts that will never successfully connect. This improves the overall reliability of the service by focusing on accounts that can actually connect to Steam.

## Blacklist File Format

The blacklist is stored in a text file with the following format:

```
username:reason
```

Where:

- `username` is the Steam account username
- `reason` is one of:
  - `banned` - Account is banned or disabled by Steam
  - `steamguard` - Account requires Steam Guard or has Steam Guard misconfigured
  - `credentials` - Account has invalid credentials (wrong password, etc.)
  - `unknown` - Unknown reason

Example:

```
steamuser1:banned
steamuser2:steamguard
steamuser3:credentials
```

## Automatic Blacklisting

Accounts are automatically blacklisted when they encounter specific login failures:

1. **Banned Accounts**

   - Accounts that are banned, suspended, or disabled by Steam
   - Error codes: `Banned`, `AccountDisabled`, `AccountLockedDown`, `Suspended`, `AccountLimitExceeded`

2. **Steam Guard Issues**

   - Accounts that require Steam Guard or have Steam Guard misconfigured
   - Error codes: `AccountLogonDeniedNoMail`, `AccountLogonDeniedVerifiedEmailRequired`, `AccountLoginDeniedNeedTwoFactor`, `TwoFactorCodeMismatch`, `TwoFactorActivationCodeMismatch`

3. **Invalid Credentials**
   - Accounts with incorrect passwords or other credential issues
   - Error codes: `InvalidPassword`, `AccountLogonDenied`, `IllegalPassword`

## Throttled Logins

Accounts that encounter login throttling (`AccountLoginDeniedThrottle`, `RateLimitExceeded`) are NOT blacklisted. Instead, the service will retry these accounts with a different proxy.

## Managing the Blacklist

### Environment Variable

The blacklist file location is specified by the `BLACKLIST_PATH` environment variable. If not set, it defaults to `blacklist.txt` in the current directory.

### Removing Accounts from the Blacklist

To remove an account from the blacklist, simply edit the blacklist file and remove the corresponding line. The service will load the updated blacklist on the next startup.

### Manually Adding Accounts to the Blacklist

You can manually add accounts to the blacklist by adding lines to the blacklist file in the format described above.

## Implementation Details

The blacklist functionality is implemented in the following components:

1. **Loading the Blacklist**

   - The `loadBlacklist()` function reads the blacklist file and populates an in-memory map of blacklisted accounts.
   - This is called during service startup before loading accounts.

2. **Checking for Blacklisted Accounts**

   - The `isBlacklisted()` function checks if an account is in the blacklist.
   - This is used during account loading to skip blacklisted accounts.

3. **Adding Accounts to the Blacklist**

   - The `blacklistAccount()` function adds an account to the blacklist and saves the updated blacklist to disk.
   - This is called when a login failure is detected that warrants blacklisting.

4. **Handling Login Failures**
   - The login failure handler in the `connect()` method detects specific error codes and blacklists accounts accordingly.
   - For throttled logins, it retries with a different proxy instead of blacklisting.
