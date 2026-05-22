const DEMO_USERNAME = 'admin';
const DEMO_PASSWORD = 'demo';

export function isDemoLogin(username: string, password: string): boolean {
  return username.trim() === DEMO_USERNAME && password === DEMO_PASSWORD;
}
