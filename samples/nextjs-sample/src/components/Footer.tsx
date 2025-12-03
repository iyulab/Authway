export default function Footer() {
  return (
    <footer className="footer">
      <p>
        <strong>Authway Next.js Sample</strong> |
        Auth Backend: <code>http://localhost:8081</code>
      </p>
      <p className="footer-links">
        <a href="http://localhost:3000" target="_blank" rel="noopener noreferrer">Admin Dashboard</a>
        <span>•</span>
        <a href="http://localhost:3001" target="_blank" rel="noopener noreferrer">Login UI</a>
        <span>•</span>
        <a href="http://localhost:8025" target="_blank" rel="noopener noreferrer">MailHog</a>
      </p>
    </footer>
  )
}
