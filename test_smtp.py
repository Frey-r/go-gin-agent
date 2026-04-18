import smtplib
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart

print('Script start')

smtp_server = 'smtp.gmail.com'
smtp_port = 587
from_email = 'eduardo.bachmann.f@gmail.com'
to_email = 'freyrimpresiones@gmail.com'
password = 'ruyp pwgm lbxd yjgt'

msg = MIMEMultipart()
msg['From'] = from_email
msg['To'] = to_email
msg['Subject'] = 'Test 2 Crustaceo Prospects MCP 🦀'

body = """
Hola equipo Freyr Impresiones,

Test 2 mensajería outbound desde OpenClaw prospects-mcp.
SMTP Gmail 100% OK.

Ready for cold outreach a leads (Mobile Outfitters, etc).

Saludos,
Eduardo / Crustaceo Agent
"""

msg.attach(MIMEText(body, 'plain'))

try:
    print('Connecting SMTP...')
    server = smtplib.SMTP(smtp_server, smtp_port)
    server.starttls()
    print('TLS OK, login...')
    server.login(from_email, password)
    print('Login OK, sending...')
    text = msg.as_string()
    server.sendmail(from_email, to_email, text)
    server.quit()
    print('Email SENT successfully to ' + to_email + '!')
except Exception as e:
    print('Error:', str(e))
