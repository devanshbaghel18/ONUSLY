import { Suspense, lazy } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import Auth from './pages/Auth';
import ProtectedRoute from './components/ProtectedRoute';
import GlobalChatNotifier from './components/GlobalChatNotifier';
import ErrorBoundary from './components/ErrorBoundary';
import { GoogleOAuthProvider } from '@react-oauth/google';

const Dashboard = lazy(() => import('./pages/Dashboard'));
const Goals = lazy(() => import('./pages/Goals'));
const GoalDetail = lazy(() => import('./pages/GoalDetail'));
const Communities = lazy(() => import('./pages/Communities'));
const ApprovalCenter = lazy(() => import('./pages/ApprovalCenter'));
const DigitalWellbeing = lazy(() => import('./pages/DigitalWellbeing'));
const Legal = lazy(() => import('./pages/Legal'));

function RouteLoader() {
  return (
    <div className="min-h-screen bg-[#0c0e11] flex items-center justify-center font-mono text-xs uppercase tracking-widest text-neutral-400">
      Loading workspace...
    </div>
  );
}

function App() {
  const clientId =
    import.meta.env.VITE_GOOGLE_CLIENT_ID ||
    '24284302254-n26lia4s6vi33koas28u1i26ejimdfp3.apps.googleusercontent.com';

  return (
    <ErrorBoundary>
      <GoogleOAuthProvider clientId={clientId}>
        <Router>
          <GlobalChatNotifier />
          <Suspense fallback={<RouteLoader />}>
            <Routes>
              <Route path="/" element={<Auth />} />
              <Route path="/auth" element={<Auth />} />
              <Route path="/privacy" element={<Legal mode="privacy" />} />
              <Route path="/terms" element={<Legal mode="terms" />} />
              <Route
                path="/dashboard"
                element={
                  <ProtectedRoute>
                    <Dashboard />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/goals"
                element={
                  <ProtectedRoute>
                    <Goals />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/goals/:id"
                element={
                  <ProtectedRoute>
                    <GoalDetail />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/community-friends"
                element={
                  <ProtectedRoute>
                    <Communities />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/approvals"
                element={
                  <ProtectedRoute>
                    <ApprovalCenter />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/wellbeing"
                element={
                  <ProtectedRoute>
                    <DigitalWellbeing />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/communities"
                element={
                  <ProtectedRoute>
                    <Communities />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/submit-proof"
                element={<Navigate to="/community-friends" replace />}
              />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Suspense>
        </Router>
      </GoogleOAuthProvider>
    </ErrorBoundary>
  );
}

export default App;
