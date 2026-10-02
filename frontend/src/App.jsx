import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { useState, useEffect } from 'react';
import Auth from './pages/Auth';
import Dashboard from './pages/Dashboard';
import ProtectedRoute from './components/ProtectedRoute';
import { GoogleOAuthProvider } from '@react-oauth/google';

function CursorGlow() {
  const [pos, setPos] = useState({ x: -100, y: -100 });

  useEffect(() => {
    const handleMouseMove = (e) => setPos({ x: e.clientX, y: e.clientY });
    window.addEventListener("mousemove", handleMouseMove);
    return () => window.removeEventListener("mousemove", handleMouseMove);
  }, []);

  return (
    <div
      className="pointer-events-none fixed top-0 left-0 z-50 h-[200px] w-[200px] rounded-full bg-blue-400/5 blur-[50px] transition-transform duration-75 ease-out"
      style={{ transform: `translate(${pos.x}px, ${pos.y}px) translate(-50%, -50%)` }}
    />
  );
}

function App() {
  const clientId =
    import.meta.env.VITE_GOOGLE_CLIENT_ID ||
    '24284302254-n26lia4s6vi33koas28u1i26ejimdfp3.apps.googleusercontent.com';

  return (
    <GoogleOAuthProvider clientId={clientId}>
      <CursorGlow />
      <Router>
        <Routes>
          <Route path="/" element={<Auth />} />
          <Route
            path="/dashboard"
            element={
              <ProtectedRoute>
                <Dashboard />
              </ProtectedRoute>
            }
          />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Router>
    </GoogleOAuthProvider>
  );
}

export default App;