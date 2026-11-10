import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@/index.css'
import App from '@/App'
import { initToken } from '@/lib/api'

// Token #hash'ten alınıp adres çubuğundan silinir; sonra uygulama başlar.
initToken()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
