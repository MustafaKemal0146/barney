import { useEffect } from 'react'
import { Shell } from '@/components/Shell'
import { useApp } from '@/store/app'

export default function App() {
  useEffect(() => {
    void useApp.getState().init()
    return useApp.getState().startSSE()
  }, [])
  return <Shell />
}
