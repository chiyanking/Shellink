import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { preference } from './backend'
import './desktop.css'
import './review.css'
import './aurora.css'

// Apply the persisted scheme before React mounts. Child effects construct the
// terminal (and read CSS custom properties) before App's effect toggles `.dark`,
// which otherwise leaves the first terminal painted with the light palette.
if (preference<'light' | 'dark'>('theme', 'light') === 'dark') {
  document.documentElement.classList.add('dark')
}

ReactDOM.createRoot(document.getElementById('app')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
