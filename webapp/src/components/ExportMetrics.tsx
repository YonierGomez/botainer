import { useState } from 'react'

interface ExportMetricsProps {
  onClose: () => void
  getAuthHeaders: () => Record<string, string>
}

export default function ExportMetrics({ onClose, getAuthHeaders }: ExportMetricsProps) {
  const [duration, setDuration] = useState('24h')
  const [format, setFormat] = useState('json')
  const [exporting, setExporting] = useState(false)

  const handleExport = async () => {
    setExporting(true)
    try {
      const response = await fetch(`/api/metrics/export?duration=${duration}&format=${format}`, {
        headers: getAuthHeaders()
      })
      
      const blob = await response.blob()
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `metrics-${Date.now()}.${format}`
      document.body.appendChild(a)
      a.click()
      window.URL.revokeObjectURL(url)
      document.body.removeChild(a)
      
      onClose()
    } catch (err) {
      console.error('Error exporting metrics:', err)
      alert('Failed to export metrics')
    } finally {
      setExporting(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4">
      <div className="bg-gray-900 w-full max-w-md rounded-2xl shadow-2xl border border-gray-700">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-gray-700">
          <div className="flex items-center gap-3">
            <span className="text-2xl">📥</span>
            <h2 className="text-lg font-bold text-white">Export Metrics</h2>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
          >
            <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Content */}
        <div className="p-6 space-y-6">
          {/* Duration */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">Time Range</label>
            <select
              value={duration}
              onChange={(e) => setDuration(e.target.value)}
              className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-xl text-white focus:outline-none focus:border-blue-500"
            >
              <option value="1h">Last Hour</option>
              <option value="24h">Last 24 Hours</option>
              <option value="168h">Last 7 Days</option>
              <option value="720h">Last 30 Days</option>
            </select>
          </div>

          {/* Format */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">Format</label>
            <div className="grid grid-cols-2 gap-2">
              <button
                onClick={() => setFormat('json')}
                className={`px-4 py-3 rounded-xl font-semibold transition-colors ${
                  format === 'json'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-800 text-gray-400 hover:bg-gray-700'
                }`}
              >
                JSON
              </button>
              <button
                onClick={() => setFormat('csv')}
                className={`px-4 py-3 rounded-xl font-semibold transition-colors ${
                  format === 'csv'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-800 text-gray-400 hover:bg-gray-700'
                }`}
              >
                CSV
              </button>
            </div>
          </div>

          {/* Info */}
          <div className="bg-blue-900/20 border border-blue-700/50 rounded-xl p-4">
            <p className="text-sm text-blue-300">
              📊 Export includes CPU and memory metrics for all containers in the selected time range.
            </p>
          </div>
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-gray-700 flex gap-2">
          <button
            onClick={handleExport}
            disabled={exporting}
            className="flex-1 px-4 py-3 bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {exporting ? 'Exporting...' : '📥 Export'}
          </button>
          <button
            onClick={onClose}
            className="flex-1 px-4 py-3 bg-gray-800 text-white rounded-xl font-semibold hover:bg-gray-700 transition-colors"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  )
}
