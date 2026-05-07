import { useState, useEffect } from 'react'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'

interface MetricPoint {
  timestamp: number
  container_id: string
  container_name: string
  cpu_percent: number
  memory_usage: number
  memory_limit: number
  memory_percent: number
}

interface HistoricalChartsProps {
  containerId: string
  containerName: string
  onClose: () => void
  getAuthHeaders: () => Record<string, string>
}

export default function HistoricalCharts({ containerId, containerName, onClose, getAuthHeaders }: HistoricalChartsProps) {
  const [metrics, setMetrics] = useState<MetricPoint[]>([])
  const [loading, setLoading] = useState(true)
  const [duration, setDuration] = useState('1h')

  useEffect(() => {
    fetchMetrics()
  }, [duration])

  const fetchMetrics = async () => {
    setLoading(true)
    try {
      const response = await fetch(`/api/containers/${containerId}/metrics?duration=${duration}`, {
        headers: getAuthHeaders()
      })
      const result = await response.json()
      
      if (result.success) {
        setMetrics(result.data || [])
      }
    } catch (err) {
      console.error('Error fetching metrics:', err)
    } finally {
      setLoading(false)
    }
  }

  const formatTimestamp = (timestamp: number) => {
    const date = new Date(timestamp * 1000)
    if (duration === '1h') {
      return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })
    } else if (duration === '24h') {
      return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })
    } else {
      return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
    }
  }

  const chartData = metrics.map(m => ({
    time: formatTimestamp(m.timestamp),
    cpu: parseFloat(m.cpu_percent.toFixed(2)),
    memory: parseFloat(m.memory_percent.toFixed(2))
  }))

  return (
    <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
      <div className="bg-gray-900 w-full sm:max-w-6xl sm:rounded-2xl shadow-2xl border border-gray-700 flex flex-col max-h-screen sm:max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-gray-700">
          <div className="flex items-center gap-3">
            <span className="text-2xl">📊</span>
            <div>
              <h2 className="text-lg font-bold text-white">Historical Charts</h2>
              <p className="text-sm text-gray-400">{containerName}</p>
            </div>
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

        {/* Duration Selector */}
        <div className="p-4 border-b border-gray-700">
          <div className="flex gap-2">
            {['1h', '24h', '168h'].map((d) => (
              <button
                key={d}
                onClick={() => setDuration(d)}
                className={`px-4 py-2 rounded-xl font-semibold transition-colors ${
                  duration === d
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-800 text-gray-400 hover:bg-gray-700'
                }`}
              >
                {d === '1h' ? 'Last Hour' : d === '24h' ? 'Last 24h' : 'Last 7 Days'}
              </button>
            ))}
          </div>
        </div>

        {/* Charts */}
        <div className="flex-1 overflow-auto p-6">
          {loading ? (
            <div className="flex items-center justify-center h-full">
              <div className="text-center">
                <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-blue-500 border-t-transparent"></div>
                <p className="mt-4 text-gray-400">Loading metrics...</p>
              </div>
            </div>
          ) : chartData.length === 0 ? (
            <div className="text-center py-12">
              <div className="text-6xl mb-4">📊</div>
              <p className="text-gray-400">No metrics available yet</p>
              <p className="text-sm text-gray-500 mt-2">Metrics are collected every 30 seconds</p>
            </div>
          ) : (
            <div className="space-y-8">
              {/* CPU Chart */}
              <div>
                <h3 className="text-lg font-bold text-white mb-4">CPU Usage (%)</h3>
                <ResponsiveContainer width="100%" height={250}>
                  <LineChart data={chartData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                    <XAxis 
                      dataKey="time" 
                      stroke="#9CA3AF"
                      style={{ fontSize: '12px' }}
                    />
                    <YAxis 
                      stroke="#9CA3AF"
                      style={{ fontSize: '12px' }}
                      domain={[0, 100]}
                    />
                    <Tooltip 
                      contentStyle={{ 
                        backgroundColor: '#1F2937', 
                        border: '1px solid #374151',
                        borderRadius: '8px'
                      }}
                      labelStyle={{ color: '#F3F4F6' }}
                    />
                    <Legend />
                    <Line 
                      type="monotone" 
                      dataKey="cpu" 
                      stroke="#3B82F6" 
                      strokeWidth={2}
                      dot={false}
                      name="CPU %"
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>

              {/* Memory Chart */}
              <div>
                <h3 className="text-lg font-bold text-white mb-4">Memory Usage (%)</h3>
                <ResponsiveContainer width="100%" height={250}>
                  <LineChart data={chartData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                    <XAxis 
                      dataKey="time" 
                      stroke="#9CA3AF"
                      style={{ fontSize: '12px' }}
                    />
                    <YAxis 
                      stroke="#9CA3AF"
                      style={{ fontSize: '12px' }}
                      domain={[0, 100]}
                    />
                    <Tooltip 
                      contentStyle={{ 
                        backgroundColor: '#1F2937', 
                        border: '1px solid #374151',
                        borderRadius: '8px'
                      }}
                      labelStyle={{ color: '#F3F4F6' }}
                    />
                    <Legend />
                    <Line 
                      type="monotone" 
                      dataKey="memory" 
                      stroke="#10B981" 
                      strokeWidth={2}
                      dot={false}
                      name="Memory %"
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-gray-700 flex gap-2">
          <button
            onClick={fetchMetrics}
            className="flex-1 px-4 py-3 bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors"
          >
            🔄 Refresh
          </button>
          <button
            onClick={onClose}
            className="flex-1 px-4 py-3 bg-gray-800 text-white rounded-xl font-semibold hover:bg-gray-700 transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
