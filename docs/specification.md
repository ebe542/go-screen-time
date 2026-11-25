# Screen Time Requirements and Design

## Child accounts and allowlist

Each child uses a separate standard Windows account. Store the Windows security identifier (SID) as the account key and keep each child's rules, counters and grants separate. Parents manage one allowlist per child with entries for desktop programs and websites. Every entry is either unrestricted or time-limited. An unrestricted entry stays available during media blocks and consumes no media budget. A program or website requires an explicit matching entry even while media time is available. Accounts without a child profile are outside the child allowlist.

The desktop program scope starts with conventional executables. An app may have multiple explicitly configured executable paths for its launcher and game. Require unique path assignments within each child policy so that a foreground interval maps to one app. Validate configured paths and surface missing or changed paths in the parent menu. Windows sign-in, the desktop and required system components need a system exemption so parents can set up a child's allowlist without listing each component individually.

## Managed browsing

Authorize the browser executable through the program allowlist and apply the website entries to browsing within it. School portals, search engines and research sites can be explicitly approved without limits. A search result opens only if its destination matches a website entry. Parents can give a media site such as YouTube a time-limited entry. Unlisted destinations are blocked at all times.

The parent menu is the place to manage website entries. A managed browser extension reports the active site to the local service and enforces access decisions, including switching an open tab to a block page when access ends. Use Firefox as a candidate for the first Windows feasibility test. Browser setup belongs to Windows administration. Verify that child policies apply to the intended accounts and cover private windows or prevent their use. Also verify that the browser stays available for approved sites and that a restricted site cannot remain usable in an open tab after its time runs out.

## Usage accounting

Count elapsed time while a time-limited app is in the foreground of an unlocked, active child session. In a managed browser, the active tab determines the website entry and the browser window must be in the foreground. Foreground time counts while the child is away from the keyboard. Background windows and tabs accrue no time. Count each interval once toward the child's shared media budget and once toward the current entry's daily total. Unrestricted use consumes no media time. Lock, sleep, sign-out and session switches stop charging the previous session. Missing observations are recorded as gaps and are excluded from totals.

Use elapsed time for measuring duration and a configured local time zone for calendar rules. Split intervals at calendar and policy boundaries. Budgets reset at local midnight, using the new day's weekday or weekend allowance. Persist counters, break state and bonus grants across service and machine restarts. Store timestamps in UTC and display them in the policy time zone.

## Daily limits and allowed hours

Set a shared daily media budget for weekdays and a separate one for weekends. Time-limited programs and websites draw from that budget and can have individual daily caps. Configure allowed hours for weekdays and weekends. Treat each window as inclusive at the start and exclusive at the end. An overnight schedule can use separate intervals on either side of midnight.

A time-limited entry is available while its schedule is open, no required break is active and it has time remaining. Regular usage requires time in both the shared budget and any configured individual cap. An app-specific bonus can cover usage of that app once either limit runs out. At the end of a window, block the entries covered by that schedule. Unrestricted entries and the Windows desktop remain available.

For example, a policy grants two hours on weekdays, three hours on weekends and use from 14:00 to 19:00. If a game has a 60-minute daily cap, playing it for an hour exhausts its allowance and leaves one hour of weekday budget available for other managed apps. The required break may temporarily suspend access before either daily limit is reached.

## Required breaks

With a 45-minute usage threshold and a 10-minute break, accumulate foreground use across all time-limited entries. A continuous 10-minute interval away from time-limited use clears the accumulated usage. Shorter interruptions preserve it. Once the threshold is reached, start a 10-minute block on all time-limited entries. Unrestricted apps and websites remain available during the break. The break elapses while the session is locked or the computer is asleep. At the end of the break, the usage counter for the next break starts at zero.

## Bonus time

Parents can grant minutes immediately in the password-protected parent menu on the child's PC. Away from the PC, either parent can create a signed and encrypted one-time message on their phone and send it to the child through a messenger. The child pastes the message into the application on the target PC, which processes it offline.

A shared grant increases the day's shared allowance. An app grant adds minutes usable only by that app, including after either daily limit is exhausted. Use these minutes only when the app would otherwise be blocked by a daily limit. Include that usage in actual usage totals and track bonus-funded minutes separately from regular budget consumption. In either case, bonus time expires at local midnight of the day it is granted or redeemed. The schedule and break rules still apply.

### One-time messages

During one-time pairing, each parent's phone registers its public verification key on the child's PC and obtains the PC's public encryption key. The public keys can be transferred manually, for example by QR code or file. Each parent keeps their private signing key on their phone, and the PC keeps its private decryption key. Each parent can issue messages independently.

The message contains a unique, random grant ID, the target PC, the child account, the requested bonus duration, an optional app ID and the redemption deadline. The phone signs the message and encrypts it for the target PC. The signed and encrypted message is the one-time code. The PC needs no pre-issued code record, code inventory or live connection to a parent's phone.

On redemption, the PC decrypts the message, verifies the signature against a registered parent key and checks the target PC, the authenticated child account and the deadline. Rate-limit failed attempts. Store the grant ID and the bonus grant in the same transaction so repeated or concurrent submissions yield at most one grant, including after a restart. Re-encrypting the same grant preserves its ID. A new grant gets a new ID even when the other details match.

The tested Go core supports direct grants, issuance and offline redemption of signed and encrypted messages, and durable replay protection in SQLite. Messages use an ASCII-armored OpenPGP block with a versioned JSON payload. The implementation uses GopenPGP v3 with the RFC 9580 profile.

The remaining work covers key pairing and private key storage, the parent and child interfaces, and an offline Windows test with two parent devices. The earlier database-issued code path remains available in the Go core. Its place in the user-facing workflow is undecided.

## Windows enforcement

Use a Go Windows service for policy decisions, durable state and enforcement. Use a Go helper in each interactive session for foreground observations and child notifications. Windows services run in Session 0, so the interactive helper supplies the observations from its own session. Resolve the foreground window to a process and validate the process image, account and session before recording usage or taking enforcement action. [Microsoft Learn on interactive services](https://learn.microsoft.com/en-us/windows/win32/services/interactive-services), [GetForegroundWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getforegroundwindow), [GetWindowThreadProcessId](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getwindowthreadprocessid), [process and thread functions](https://learn.microsoft.com/en-us/windows/win32/procthread/process-and-thread-functions).

Prevent unlisted programs from launching under a child account. The Windows mechanism for launch-time enforcement still needs to be selected and validated. For a running time-limited app, warn the child before predictable limits. When a limit takes effect, request a graceful close where supported and enforce closure after a bounded grace period if the app stays open. Forced closure can lose unsaved work. Check the target with the same process handle before forced closure and scope every action to the matched executable, child account and session. Record enforcement failures and expose them to the parent.

Run the service and protect its binaries, configuration and usage database so standard child accounts cannot modify them. Authenticate and authorize local clients using Windows identity and restrict each client's operations. The service owns timekeeping and policy state. Treat session reports as observations, validate process ownership and report a missing helper as degraded monitoring. Trackable time and launch detection can have gaps if monitoring fails or a game resists closure. Local administrators can control the installed service.

## Parent menu and usage history

Grant parent access only to designated Windows accounts and require the parent password before exposing administrative operations. Store a salted password verifier, rate-limit attempts and expire inactive sessions. Authenticate administrative requests at the service boundary. Enrollment and recovery require a Windows administrator.

Show parents daily totals per child and time-limited entry, with timestamps for recorded usage intervals, breaks, denials, bonus grants and monitoring gaps. Keep data local and define retention and deletion controls before release. Use configured entry IDs for usage records and keep window titles, content, input, raw browsing URLs and plaintext secrets out of logs. Give children access to their own remaining time, the reason for a block and when they can next try the entry.

## Acceptance criteria

| Scenario | Expected outcome |
| --- | --- |
| Try an unlisted executable or website as a child | The executable cannot launch or the website cannot open, even when media time is available. The desktop and approved entries remain usable. |
| Use a time-limited app or website during an allowed period | Only foreground use updates the individual total and shared total, once per interval. |
| Exhaust an individual cap while shared time remains | The affected app or page is blocked, including subsequent attempts. Other eligible entries remain usable. |
| Exhaust the shared budget or reach the schedule end | With the shared budget exhausted, only unrestricted entries and apps with eligible app-specific bonus time remain available. At the end of a schedule window, affected apps close and affected tabs show a block page. |
| Run through a break threshold across an app and a website | Time-limited apps and websites are blocked for the configured break. Unrestricted entries stay available. A full break restores access when the other rules permit it. |
| Grant minutes in the parent menu | The selected child receives the chosen minutes immediately in the shared budget or for the selected app. Hours and break rules still apply. |
| Redeem an encrypted message from either parent | The intended child can redeem a valid message on the target PC offline. Messages for another PC or account, expired or altered messages, and messages with an unregistered signer yield no grant. |
| Replay a grant after redemption | Repeated and concurrent submissions, including a re-encrypted copy or a submission after restart, yield no additional time for the same grant ID. |
| Switch between two child accounts | Each account sees its own totals and grants. The inactive session accrues no time. |
| Cross midnight, a weekday/weekend boundary or a daylight-saving transition | Daily totals are attributed to the correct local date, the new allowance applies and elapsed usage is counted once. |
| Lock, sleep, restart or lose the session helper | Charges stop during unobserved periods. Previously committed totals and grants survive, and monitoring gaps are visible. |
| Run a similarly named executable or the same app under a different account | Enforcement acts only on the configured executable for the child covered by the rule. |
| Attempt a policy change from a child session or without the parent password | The service denies the operation and policy remains intact. |

Cover the policy rules with deterministic Go tests using controlled timestamps. Verify Windows integration separately with standard child accounts and disposable desktop programs, including launch and relaunch, account switching, lock, sleep and restart. Verify each supported game against the closure behavior before publishing a compatibility claim.
