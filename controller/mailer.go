package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

func sendPlainTextMail(settings MailSettings, toEmail string, subject string, body string) error {
	host := strings.TrimSpace(settings.SMTPHost)
	port := settings.SMTPPort
	user := strings.TrimSpace(settings.SMTPUser)
	pass := strings.TrimSpace(settings.SMTPPass)
	fromEmail := strings.TrimSpace(settings.FromEmail)
	fromName := strings.TrimSpace(settings.FromName)
	toEmail = strings.TrimSpace(toEmail)

	if host == "" || port <= 0 || fromEmail == "" || toEmail == "" {
		return fmt.Errorf("SMTP 配置不完整")
	}
	if fromName == "" {
		fromName = "GPU Ops 团队"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", fromName, fromEmail))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", toEmail))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	msg.WriteString("\r\n")

	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}

	// 465 端口通常为隐式 TLS，需要先建 TLS 连接再 SMTP 握手。
	if port == 465 {
		tlsConn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		defer tlsConn.Close()
		c, err := smtp.NewClient(tlsConn, host)
		if err != nil {
			return err
		}
		defer c.Close()
		if auth != nil {
			if ok, _ := c.Extension("AUTH"); ok {
				if err := c.Auth(auth); err != nil {
					return err
				}
			}
		}
		if err := c.Mail(fromEmail); err != nil {
			return err
		}
		if err := c.Rcpt(toEmail); err != nil {
			return err
		}
		wc, err := c.Data()
		if err != nil {
			return err
		}
		if _, err := wc.Write(msg.Bytes()); err != nil {
			_ = wc.Close()
			return err
		}
		if err := wc.Close(); err != nil {
			return err
		}
		return c.Quit()
	}

	// 非 465 端口优先尝试标准 SendMail（支持 STARTTLS 的服务端会自动协商）。
	if err := smtp.SendMail(addr, auth, fromEmail, []string{toEmail}, msg.Bytes()); err == nil {
		return nil
	}

	// 某些服务端需要明确 STARTTLS，这里追加一次显式握手尝试。
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := c.Mail(fromEmail); err != nil {
		return err
	}
	if err := c.Rcpt(toEmail); err != nil {
		return err
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg.Bytes()); err != nil {
		_ = wc.Close()
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func sendResetPasswordMail(settings MailSettings, toEmail string, subject string, body string) error {
	return sendPlainTextMail(settings, toEmail, subject, body)
}
