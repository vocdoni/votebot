# VoteBot: onvote Farcaster Bot

Simple [Warpcast](https://warpcast.com/) bot to create polls frames using [farcaster.vote](https://farcaster.vote/app), an [onvote](https://onvote.app/) experiment.

## Usage

### Requirements

* Go (>= 1.21.7)
* Warpcast account for the bot.
* Farcaster Hub HTTP Enpoint (**optional, a public one is used by default*).

### Basic example 

```sh
go run cmd/votebot/main.go \
    -botFid <existing_user_id> \
    -privateKey <user_signer_private_key>
```

### Use your own hub

```sh
go run cmd/votebot/main.go \
    -botFid <existing_user_id> \
    -privateKey <user_signer_private_key> \
    -endpoint <farcaster_hub_http_url>
```

For example, `debug` over [neynar hub](https://neynar.com/):

```sh
go run cmd/votebot/main.go \
    -logLevel debug \
     -botFid <existing_user_id> \
    -privateKey <user_signer_private_key> \
    -endpoint https://hub-api.neynar.com/v1 \
    -authHeaders api_key \
    -authKeys <neynar_api_key>
```

#### Creating a new signer to your FID

The bot will answer to the users with the result of their requests, so it needs the private key of a registered signer for it FID. This signer private key is used to sign bot messages. 

To register a new signer and get its private key, follow these steps:

1. Go to the official [frames debugger](https://debugger.framesjs.org/debug) and complete the login with Warpcast.
    1. Click on `Sign in with farcaster (costs warps once, works with remote frames and other libs)` option.
    1. The web app will generate a QR code that you must to scan with a mobile phone with Warpcast installed ([official farcaster client](https://www.farcaster.xyz/)) and logged in the bot account.
    2. If the QR does not work, copy the the link address of the `open url` option and paste it in your phone browser. Ensure that the address is directly accessed and not entered in any search engine.
    3. The Warpcast will be openned to confirm the signer creation (it costs a few wraps).
2. Return to the web app and open the `dev-tools`. You will find all the signer information (including its private key) in the local storage.
