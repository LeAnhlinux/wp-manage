import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './theme-ops.css' // Ops Console theme — must load after Tailwind so its overrides win
import App from './App.tsx'

// Apply the saved theme before first paint so there's no dark→light flash.
// Dark is the default; 'light' adds the html.theme-light override layer.
if (localStorage.getItem('wpm-theme') === 'light') {
  document.documentElement.classList.add('theme-light')
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
