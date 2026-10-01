import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import Auth from './pages/Auth';
import { GoogleOAuthProvider } from '@react-oauth/google';

function App() {
  const clientId = import.meta.env.VITE_GOOGLE_CLIENT_ID;

  return (
    <GoogleOAuthProvider clientId={clientId}>
      <Router>
        <Routes>
          <Route path="/" element={<Auth />} />
        </Routes>
      </Router>
    </GoogleOAuthProvider>
  );
}

export default App;