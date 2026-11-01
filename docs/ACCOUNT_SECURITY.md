# Account access and deletion

Available controls are in **Account & security**. This is personal-account
recovery using a saved code; no email sender, paid identity service, or bank
connection is required. It does not verify ownership of an email inbox.

## Recovery code

While signed in, enter the current password and generate a recovery code.
Download it or save it in a private password manager, separately from the login
password. The app shows it once and stores only a SHA-256 hash. It is not part
of the financial workspace JSON backup or shared browser-test artifacts.

Generating another code immediately replaces the previous one. If the response
is interrupted before the code is saved, generate a new code using the password;
the previous code may already have been replaced. A configured status means a
code exists, not that the user successfully saved it.

From login, choose **Forgot your password?**, enter the email, saved code, and
new password. A successful recovery consumes the code atomically and ends all
access/refresh sessions. Concurrent uses of one code cannot both succeed. Log
in again and save a new code. Without a previously saved code, the current app
cannot reset a forgotten password. A financial backup can recover records into
a new empty workspace, but cannot recover an old login.

## Password change and sessions

Changing a password requires the current password, ends all sessions, and keeps
an existing unused recovery code valid. API requests check the user's current
credential version in PostgreSQL; password changes/recovery invalidate old
access JWTs immediately, in addition to revoking refresh tokens. This adds one
indexed user lookup per protected request and must be included in load tests.
The login path rechecks the password-hash generation under the auth lock so a
concurrent reset cannot issue a valid session using a checked old password.

## Account deletion

The user can download a financial backup first, review the deletion explanation,
enter the current password, and type `DELETE`. Cancellation and failed password
checks leave the workspace intact. Successful deletion atomically removes the
user and owning financial records, imports, corrections, categories, report
metadata, saved checks, credentials, and unprocessed outbox events. Other users
remain separate. Old access/refresh credentials cannot access a deleted account.

Generated CSV objects and retained Redis ledger events use durable PostgreSQL
cleanup queues, retried in bounded background batches. An export job holds a
shared lease until it finishes storing its file; deletion waits on the matching
exclusive lease before queuing its object key. This prevents a late writer from
recreating a private report after cleanup. Object-store failures do not prevent
independent event cleanup. The owning-user deletion cascade does not weaken
the normal append-only protection of transactions and postings.

Derived Redis account summaries expire within five minutes. Queue/rate-limit
metadata can contain numeric IDs until its normal expiry. Files downloaded onto
a device and server backups are separate copies. Generated file/event cleanup
requires a running app and available stores; no exact completion time is
promised during an outage. S3-compatible versioning, replication, lifecycle
retention, and server backup retention must be managed by the deployment owner;
an object delete does not erase historical bucket versions.

The server data restore command invalidates restored sessions. Restoring older
backups can also restore users who deleted their accounts later; handle those
requests before reopening a recovered service. The app does not automatically
erase operator-maintained backup copies.

## Endpoints

| Method | Path | Behavior |
| --- | --- | --- |
| GET | `/v1/security` | Current email and whether a recovery code is configured; never returns its hash/code. |
| POST | `/v1/security/recovery-code` | `current_password`; reveals a replacement `recovery_code` once. |
| POST | `/v1/security/password` | `current_password`, `password`; ends all sessions. |
| POST | `/auth/recover` | `email`, `recovery_code`, `password`; consumes the code and ends all sessions. |
| DELETE | `/v1/security/account` | `current_password`, `confirmation: "DELETE"`; deletes live records and queues cleanup. |

All `/v1` routes require a current bearer session. `/auth/recover` uses the
existing per-IP authentication limiter. Incorrect current passwords return 403
so the browser does not attempt session refresh or log out a valid user merely
for mistyping. Recovery failures return a generic credential/code error. Auth
and account-security responses use `Cache-Control: no-store`. Requests are
bounded; recovery codes/passwords must not appear in server logs or URLs.
