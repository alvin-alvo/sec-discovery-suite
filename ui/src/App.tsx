import React from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Navbar } from './components/Navbar'
import { RunsPage } from './pages/RunsPage'
import { FSReportPage } from './pages/FSReportPage'
import { TLSReportPage } from './pages/TLSReportPage'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

export const App: React.FC = () => {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <div className="min-h-screen flex flex-col bg-slate-950 text-slate-100 selection:bg-cyan-500 selection:text-white">
          <Navbar />
          <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8">
            <Routes>
              <Route path="/" element={<Navigate to="/runs" replace />} />
              <Route path="/runs" element={<RunsPage />} />
              <Route path="/runs/:id/fs" element={<FSReportPage />} />
              <Route path="/runs/:id/tls" element={<TLSReportPage />} />
              <Route path="*" element={<Navigate to="/runs" replace />} />
            </Routes>
          </main>
        </div>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
