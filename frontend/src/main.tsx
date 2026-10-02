import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import SoundFX from './components/SoundFX';
import Toast from './components/Toast';
import { initTheme } from './theme';
import './styles/themes.css';
import './styles/app.css';
import './styles/liquid-glass.css';

initTheme();
document.body.style.margin = '0';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <SoundFX />
    <Toast />
    <App />
  </React.StrictMode>
);
