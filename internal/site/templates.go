package site

import (
	"fmt"
)

// renderHTMLWelcome generates the static HTML landing page.
func renderHTMLWelcome(domain string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Welcome to %s</title>
    <style>
        body { font-family: 'Outfit', sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.7); padding: 3rem; border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.1); text-align: center; max-width: 500px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #38bdf8; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { background: #0ea5e9; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.85rem; font-weight: bold; color: white; display: inline-block; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">AgilePanel HTML</div>
        <h1>%s</h1>
        <p>Your static HTML website has been successfully provisioned and is online.</p>
        <p style="font-size: 0.9rem; color: #64748b;">Upload your static assets to <code>/var/www/%s/htdocs</code> to replace this page.</p>
    </div>
</body>
</html>`, domain, domain, domain)
}

// renderPHPWelcome generates the landing page for custom PHP sites without exposing database passwords.
func renderPHPWelcome(domain string, dbName string, dbUser string) string {
	return fmt.Sprintf(`<?php
$domain = $_SERVER['HTTP_HOST'];
?>
<!DOCTYPE html>
<html>
<head>
    <title>Welcome to <?php echo htmlspecialchars($domain); ?></title>
    <style>
        body { font-family: 'Outfit', sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.7); padding: 3rem; border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.1); text-align: center; max-width: 500px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #10b981; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { background: #10b981; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.85rem; font-weight: bold; color: white; display: inline-block; }
        ul { text-align: left; background: #1e293b; padding: 1.5rem 1.5rem 1.5rem 2.5rem; border-radius: 8px; border: 1px solid rgba(255,255,255,0.05); color: #cbd5e1; font-family: monospace; }
        li { margin-bottom: 0.5rem; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">AgilePanel PHP</div>
        <h1><?php echo htmlspecialchars($domain); ?></h1>
        <p>Your custom PHP environment is fully configured and online.</p>
        
        <h3>Database Configuration</h3>
        <ul>
            <li><strong>Host:</strong> localhost (UNIX Socket)</li>
            <li><strong>Database:</strong> %s</li>
            <li><strong>Username:</strong> %s</li>
            <li><strong>Password:</strong> <em>[Set during site creation; query via 'ap site info' as root]</em></li>
        </ul>

        <p style="font-size: 0.9rem; color: #64748b; margin-top: 2rem;">Webroot is at <code>/var/www/<?php echo htmlspecialchars($domain); ?>/htdocs</code></p>
    </div>
</body>
</html>`, dbName, dbUser)
}

// renderLaravelWelcome generates the Laravel landing page without embedding plaintext passwords.
func renderLaravelWelcome(domain string, dbName string, dbUser string) string {
	return fmt.Sprintf(`<?php
$domain = $_SERVER['HTTP_HOST'];
?>
<!DOCTYPE html>
<html>
<head>
    <title>Laravel Ready - <?php echo htmlspecialchars($domain); ?></title>
    <style>
        body { font-family: 'Outfit', sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.7); padding: 3rem; border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.1); text-align: center; max-width: 600px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #f43f5e; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { background: #f43f5e; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.85rem; font-weight: bold; color: white; display: inline-block; }
        pre { text-align: left; background: #020617; padding: 1.5rem; border-radius: 8px; border: 1px solid rgba(255,255,255,0.05); color: #38bdf8; font-size: 0.9rem; overflow-x: auto; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">AgilePanel Laravel Ready</div>
        <h1>%s</h1>
        <p>Your server environment is fully configured for Laravel.</p>
        <p>To deploy your Laravel application, configure your <code>.env</code> file in <code>/var/www/%s/htdocs</code> with these settings:</p>
        
        <pre>DB_CONNECTION=mysql
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=%s
DB_USERNAME=%s
DB_PASSWORD=YOUR_DB_PASSWORD</pre>

        <p style="font-size: 0.85rem; color: #94a3b8; margin-top: 1rem;">Note: Replace <code>YOUR_DB_PASSWORD</code> with the password provided during site creation or retrieved via <code>ap site info %s</code>.</p>
        <p style="font-size: 0.9rem; color: #64748b; margin-top: 1.5rem;">Caddy is currently serving from: <code>/var/www/%s/htdocs/public</code></p>
    </div>
</body>
</html>`, domain, domain, dbName, dbUser, domain, domain)
}
