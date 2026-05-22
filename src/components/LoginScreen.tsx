import { useState } from 'react';

interface LoginScreenProps {
  onLogin: (username: string, password: string) => boolean;
}

export function LoginScreen({ onLogin }: LoginScreenProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setError('');

    // Simulate login delay for demo
    setTimeout(() => {
      const loginAccepted = onLogin(username, password);
      if (!loginAccepted) {
        setError('Use the demo credentials: admin / demo');
      }
      setIsLoading(false);
    }, 1000);
  };

  return (
    <div className="login-screen">
      <div className="login-container">
        <div className="login-header">
          <div className="login-logo">
            <div className="logo-icon">🚀</div>
            <h1>Harvester</h1>
          </div>
          <h2>Workload Wizard</h2>
          <p>Advanced Kubernetes workload and storage management</p>
        </div>

        <form className="login-form" onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="username">Username</label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={(e) => {
                setUsername(e.target.value);
                setError('');
              }}
              placeholder="Enter your username"
              required
            />
          </div>

          <div className="form-group">
            <label htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => {
                setPassword(e.target.value);
                setError('');
              }}
              placeholder="Enter your password"
              required
            />
          </div>

          {error && <div className="login-error" role="alert">{error}</div>}

          <button
            type="submit"
            className="login-btn"
            disabled={isLoading}
          >
            {isLoading ? (
              <>
                <div className="spinner-small"></div>
                Signing In...
              </>
            ) : (
              'Sign In'
            )}
          </button>
        </form>

        <div className="login-footer">
          <div className="demo-info">
            <h3>🎯 Demo Features</h3>
            <ul>
              <li>🔐 Secure login system</li>
              <li>⚙️ Advanced workload configuration</li>
              <li>💾 Multiple storage backends (Ceph, NFS, SMB, NVMe-oF, RDMA, ZFS)</li>
              <li>📝 Real-time YAML editing</li>
              <li>🚀 Automated deployment with storage installation</li>
              <li>🎨 Dark mode UI with customizable themes</li>
            </ul>
          </div>

          <div className="demo-credentials">
            <p><strong>Demo Credentials:</strong></p>
            <p>Username: <code>admin</code></p>
            <p>Password: <code>demo</code></p>
          </div>
        </div>
      </div>
    </div>
  );
}