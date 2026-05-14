import { useState, useEffect } from 'react'

interface ComposeProject {
  name: string
  path: string
  file: string
}

interface ComposeManagerProps {
  onClose: () => void
  getAuthHeaders: () => Record<string, string>
}

export default function ComposeManager({ onClose, getAuthHeaders }: ComposeManagerProps) {
  const [projects, setProjects] = useState<ComposeProject[]>([])
  const [loading, setLoading] = useState(true)
  const [executing, setExecuting] = useState<string | null>(null)

  useEffect(() => {
    fetchProjects()
  }, [])

  const fetchProjects = async () => {
    setLoading(true)
    try {
      const response = await fetch('/api/compose/projects', {
        headers: getAuthHeaders()
      })
      const result = await response.json()
      if (result.success) {
        setProjects(result.data || [])
      }
    } catch (err) {
      console.error('Error fetching projects:', err)
    } finally {
      setLoading(false)
    }
  }

  const handleAction = async (project: ComposeProject, action: string) => {
    setExecuting(`${project.name}-${action}`)
    try {
      const response = await fetch('/api/compose/action', {
        method: 'POST',
        headers: {
          ...getAuthHeaders(),
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          path: project.path,
          action
        })
      })
      const result = await response.json()
      if (result.success) {
        alert(`${action} completed successfully`)
        if (action === 'down') {
          fetchProjects()
        }
      } else {
        alert(`Failed: ${result.error}`)
      }
    } catch (err) {
      alert('Action failed')
    } finally {
      setExecuting(null)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
      <div className="bg-gray-900 w-full sm:max-w-4xl sm:rounded-2xl shadow-2xl border border-gray-700 flex flex-col max-h-screen sm:max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-gray-700">
          <div className="flex items-center gap-3">
            <span className="text-2xl">🐳</span>
            <h2 className="text-lg font-bold text-white">Docker Compose Manager</h2>
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
        <div className="flex-1 overflow-y-auto p-4">
          {loading ? (
            <div className="text-center py-12">
              <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-blue-500 border-t-transparent"></div>
              <p className="mt-4 text-gray-400">Loading projects...</p>
            </div>
          ) : projects.length === 0 ? (
            <div className="text-center py-12">
              <span className="text-6xl">📦</span>
              <p className="mt-4 text-gray-400">No Docker Compose projects found</p>
              <p className="text-sm text-gray-500 mt-2">Place compose.yaml files in /workspace</p>
            </div>
          ) : (
            <div className="space-y-4">
              {projects.map(project => (
                <div key={project.path} className="bg-gray-800 rounded-xl p-4 border border-gray-700">
                  <div className="flex items-start justify-between gap-4">
                    <div className="flex-1">
                      <h3 className="font-bold text-white text-lg mb-1">{project.name}</h3>
                      <p className="text-sm text-gray-400 mb-2">{project.path}</p>
                      <p className="text-xs text-gray-500">{project.file}</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <button
                        onClick={() => {
                          if (confirm(`Start all services in ${project.name}?`)) {
                            handleAction(project, 'up')
                          }
                        }}
                        disabled={executing === `${project.name}-up`}
                        className="px-3 py-2 bg-emerald-600 text-white rounded-lg font-semibold hover:bg-emerald-700 transition-colors text-sm disabled:opacity-50"
                      >
                        {executing === `${project.name}-up` ? '⏳' : '▶️'} Up
                      </button>
                      <button
                        onClick={() => {
                          if (confirm(`Restart all services in ${project.name}?`)) {
                            handleAction(project, 'restart')
                          }
                        }}
                        disabled={executing === `${project.name}-restart`}
                        className="px-3 py-2 bg-amber-600 text-white rounded-lg font-semibold hover:bg-amber-700 transition-colors text-sm disabled:opacity-50"
                      >
                        {executing === `${project.name}-restart` ? '⏳' : '🔄'} Restart
                      </button>
                      <button
                        onClick={() => {
                          if (confirm(`Pull all images for ${project.name}?`)) {
                            handleAction(project, 'pull')
                          }
                        }}
                        disabled={executing === `${project.name}-pull`}
                        className="px-3 py-2 bg-blue-600 text-white rounded-lg font-semibold hover:bg-blue-700 transition-colors text-sm disabled:opacity-50"
                      >
                        {executing === `${project.name}-pull` ? '⏳' : '⬇️'} Pull
                      </button>
                      <button
                        onClick={() => {
                          if (confirm(`Stop all services in ${project.name}?`)) {
                            handleAction(project, 'down')
                          }
                        }}
                        disabled={executing === `${project.name}-down`}
                        className="px-3 py-2 bg-red-600 text-white rounded-lg font-semibold hover:bg-red-700 transition-colors text-sm disabled:opacity-50"
                      >
                        {executing === `${project.name}-down` ? '⏳' : '⏹️'} Down
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-gray-700">
          <button
            onClick={onClose}
            className="w-full px-4 py-3 bg-gray-800 text-white rounded-xl font-semibold hover:bg-gray-700 transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
