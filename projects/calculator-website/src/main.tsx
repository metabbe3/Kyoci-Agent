import React from 'react';
import { createRoot } from 'react-dom/client';
import Calculator from './App';
import './index.css';

const container = document.getElementById('root');
if (!container) {
  throw new Error('Root container #root not found in document');
}

createRoot(container).render(
  <React.StrictMode>
    <Calculator />
  </React.StrictMode>
);
