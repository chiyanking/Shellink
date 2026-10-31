import React from 'react'
import ReactDOM from 'react-dom/client'
import { isTauri } from '@tauri-apps/api/core'
import App from './App'
import './desktop.css'
import './review.css'

document.documentElement.classList.toggle('tauri-runtime', isTauri())

ReactDOM.createRoot(document.getElementById('app')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
