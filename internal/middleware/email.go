package middleware

import (
	"fmt"
	"strings"
)

func emailSubject(mail mailRequest) string {
	return fmt.Sprintf("%s %s (%s) is ready to test", mail.Name, mail.Version, mail.Build)
}

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;")

func emailHTML(mail mailRequest) string {
	note := ""
	if mail.PersonalMessage != nil && *mail.PersonalMessage != "" {
		message := strings.NewReplacer("\r\n", "<br />", "\n", "<br />").Replace(htmlEscaper.Replace(*mail.PersonalMessage))
		note = strings.ReplaceAll(emailNote, "{{message}}", message)
	}
	return strings.NewReplacer(
		"{{name}}", htmlEscaper.Replace(mail.Name),
		"{{version}}", htmlEscaper.Replace(mail.Version),
		"{{build}}", htmlEscaper.Replace(mail.Build),
		"{{installURL}}", htmlEscaper.Replace(mail.InstallURL),
		"{{title}}", htmlEscaper.Replace(emailSubject(mail)),
		"{{note}}", note,
	).Replace(emailShell)
}

func emailText(mail mailRequest) string {
	lines := []string{
		emailSubject(mail), "",
		"You have been invited to test this build. Open this link on your iPhone or iPad to install it:",
		mail.InstallURL, "", "App: " + mail.Name, "Version: " + mail.Version, "Build: " + mail.Build,
	}
	if mail.PersonalMessage != nil && *mail.PersonalMessage != "" {
		lines = append(lines, "", "Message from the developer:", *mail.PersonalMessage)
	}
	lines = append(lines, "", "Sent automatically by AppBox. You received this because a developer added your email address when they uploaded this build. https://getappbox.com")
	return strings.Join(lines, "\n")
}

const emailShell = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width,initial-scale=1" />
<meta name="color-scheme" content="light" />
<title>{{title}}</title>
<style>
@media screen and (max-width:640px) {
.ab-pad { padding-left:24px !important; padding-right:24px !important; }
.ab-title { font-size:22px !important; line-height:30px !important; }
.ab-lead { font-size:15px !important; line-height:23px !important; }
.ab-stat { display:block !important; width:100% !important; padding:0 0 16px 0 !important; }
.ab-stat-last { padding-bottom:0 !important; }
.ab-btn { display:block !important; text-align:center !important; }
.ab-shell { padding:16px 12px !important; }
}
</style>
</head>
<body style="margin:0;padding:0;background-color:#f4f4f5;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;">Install {{name}} {{version}} ({{build}}) on your iPhone or iPad.</div>
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%" style="border-collapse:collapse;background-color:#f4f4f5;">
<tr><td align="center" class="ab-shell" style="padding:48px 20px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="680" style="border-collapse:collapse;width:100%;max-width:680px;background-color:#ffffff;border:1px solid #e4e4e7;border-radius:12px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;">

<tr><td class="ab-pad" style="padding:36px 48px 0 48px;">
<p style="margin:0;font-size:13px;line-height:18px;font-weight:600;letter-spacing:0.08em;text-transform:uppercase;color:#71717a;">AppBox</p>
</td></tr>

<tr><td class="ab-pad" style="padding:18px 48px 0 48px;">
<h1 class="ab-title" style="margin:0;font-size:28px;line-height:36px;font-weight:600;letter-spacing:-0.01em;color:#18181b;">{{title}}</h1>
<p class="ab-lead" style="margin:14px 0 0 0;font-size:16px;line-height:26px;color:#3f3f46;">You have been invited to test this build. Open this email on your iPhone or iPad, then tap Install to add it to your home screen.</p>
</td></tr>

<tr><td class="ab-pad" style="padding:28px 48px 0 48px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse;">
<tr><td align="center" style="border-radius:6px;background-color:#18181b;">
<a href="{{installURL}}" class="ab-btn" style="display:inline-block;padding:14px 32px;font-size:16px;line-height:20px;font-weight:600;color:#ffffff;text-decoration:none;border-radius:6px;">Install {{name}}</a>
</td></tr>
</table>
<p style="margin:18px 0 0 0;font-size:13px;line-height:20px;color:#71717a;">If the button does not work, copy this link into Safari on your iPhone or iPad:<br />
<a href="{{installURL}}" style="color:#3f3f46;text-decoration:underline;word-break:break-all;">{{installURL}}</a></p>
</td></tr>

<tr><td class="ab-pad" style="padding:32px 48px 0 48px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%" style="border-collapse:collapse;background-color:#fafafa;border:1px solid #e4e4e7;border-radius:8px;">
<tr><td style="padding:22px 26px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%" style="border-collapse:collapse;">
<tr>
<td class="ab-stat" width="40%" valign="top" style="padding:0 16px 0 0;">
<p style="margin:0 0 4px 0;font-size:11px;line-height:15px;font-weight:600;letter-spacing:0.06em;text-transform:uppercase;color:#71717a;">App</p>
<p style="margin:0;font-size:16px;line-height:22px;font-weight:600;color:#18181b;">{{name}}</p>
</td>
<td class="ab-stat" width="30%" valign="top" style="padding:0 16px 0 0;">
<p style="margin:0 0 4px 0;font-size:11px;line-height:15px;font-weight:600;letter-spacing:0.06em;text-transform:uppercase;color:#71717a;">Version</p>
<p style="margin:0;font-size:16px;line-height:22px;font-weight:600;color:#18181b;">{{version}}</p>
</td>
<td class="ab-stat ab-stat-last" width="30%" valign="top" style="padding:0;">
<p style="margin:0 0 4px 0;font-size:11px;line-height:15px;font-weight:600;letter-spacing:0.06em;text-transform:uppercase;color:#71717a;">Build</p>
<p style="margin:0;font-size:16px;line-height:22px;font-weight:600;color:#18181b;">{{build}}</p>
</td>
</tr>
</table>
</td></tr>
</table>
</td></tr>

{{note}}

<tr><td class="ab-pad" style="padding:32px 48px 36px 48px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%" style="border-collapse:collapse;border-top:1px solid #e4e4e7;">
<tr><td style="padding:22px 0 0 0;">
<p style="margin:0;font-size:12px;line-height:20px;color:#a1a1aa;">Sent automatically by <a href="https://getappbox.com" style="color:#71717a;text-decoration:underline;">AppBox</a>. You received this because a developer added your email address when they uploaded this build.</p>
</td></tr>
</table>
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>`

const emailNote = `<tr><td class="ab-pad" style="padding:20px 48px 0 48px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%" style="border-collapse:collapse;border-left:3px solid #e4e4e7;">
<tr><td style="padding:2px 0 2px 20px;">
<p style="margin:0 0 8px 0;font-size:12px;line-height:16px;font-weight:600;letter-spacing:0.04em;text-transform:uppercase;color:#71717a;">Message from the developer</p>
<p style="margin:0;font-size:15px;line-height:24px;color:#3f3f46;">{{message}}</p>
</td></tr>
</table>
</td></tr>`
