import { useState } from 'react'

interface ContainerCreatorProps {
  onClose: () => void
  onSuccess: () => void
  getAuthHeaders: () => Record<string, string>
}

export default function ContainerCreator({ onClose, onSuccess, getAuthHeaders }: ContainerCreatorProps) {
  const [name, setName] = useState('')
  const [image, setImage] = useState('')
  const [ports, setPorts] = useState('')
  const [volumes, setVolumes] = useState('')
  const [env, setEnv] = useState('')
  const [network, setNetwork] = useState('bridge')
  const [restart, setRestart] = useState('unless-stopped')
  const [creating, setCreating] = useState(false)

  const handleCreate = async () => {
    if (!name || !image) {
      alert('Name and image are required')
      return
    }

    setCreating(true)
    try {
      // Parse ports (format: "8080:80,3000:3000")
      const portsObj: Record<string, string> = {}
      if (ports) {
        ports.split(',').forEach(p => {
          const [host, container] = p.trim().split(':')
          if (host && container) {
            portsObj[`${container}/tcp`] = host
          }
        })
      }

      // Parse volumes (format: "/host:/container,/host2:/container2")
      const volumesArr = volumes ? volumes.split(',').map(v => v.trim()).filter(v => v) : []

      // Parse env (format: "KEY=value,KEY2=value2")
      const envArr = env ? env.split(',').map(e => e.trim()).filter(e => e) : []

      const response = await fetch('/api/containers', {
        method: 'POST',
        headers: {
          ...getAuthHeaders(),
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          name,
          image,
          ports: portsObj,
          volumes: volumesArr,
          env: envArr,
          network,
          restart
        })
      })

      const result = await response.json()
      if (result.success) {
        alert('Container created successfully!')
        onSuccess()
        onClose()
      } else {
        alert(`Failed: ${result.error}`)
      }
    } catch (err) {
      alert('Failed to create container')
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
      <div className="bg-gray-900 w-full sm:max-w-2xl sm:rounded-2xl shadow-2xl border border-gray-700 flex flex-col max-h-screen sm:max-h-[90vh]">
        <div className="flex items-center justify-between p-4 border-b border-gray-700">
          <div className="flex items-center gap-3">
            <span className="text-2xl">🐳</span>
            <h2 className="text-lg font-bold text-white">Create Container</h2>
          </div>
          <button onClick={onClose} className="p-2 hover:bg-gray-800 rounded-lg transition-colors">
            <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">Container Name *</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="my-container"
              className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-xl text-white focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">Image *</label>
            <input
              type="text"
              value={image}
              onChange={(e) => setImage(e.target.value)}
              placeholder="nginx:latest"
              className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-xl text-white focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">Ports (host:container)</label>
            <input
              type="text"
              value={ports}
              onChange={(e) => setPorts(e.target.value)}
              placeholder="8080:80,3000:3000"
              className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-xl text-white focus:outline-none focus:border-blue-500"
            />
            <p className="text-xs text-gray-500 mt-1">Comma-separated: 8080:80,3000:3000</p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">Volumes (host:container)</label>
            <input
              type="text"
              value={volumes}
              onChange={(e) => setVolumes(e.target.value)}
              placeholder="/host/path:/container/path"
              className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-xl text-white focus:outline-none focus:border-blue-500"
            />
            <p className="text-xs text-gray-500 mt-1">Comma-separated: /host:/container,/host2:/container2</p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">Environment Variables</label>
            <input
              type="text"
              value={env}
              onChange={(e) => setEnv(e.target.value)}
              placeholder="KEY=value,KEY2=value2"
              className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-xl text-white focus:outline-none focus:border-blue-500"
            />
            <p className="text-xs text-gray-500 mt-1">Comma-separated: KEY=value,KEY2=value2</p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">Network</label>
            <select
              value={network}
              onChange={(e) => setNetwork(e.target.value)}
              className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-xl text-white focus:outline-none focus:border-blue-500"
            >
              <option value="bridge">bridge</option>
              <option value="host">host</option>
              <option value="none">none</option>
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">Restart Policy</label>
            <select
              value={restart}
              onChange={(e) => setRestart(e.target.value)}
              className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-xl text-white focus:outline-none focus:border-blue-500"
            >
              <option value="no">no</option>
              <option value="always">always</option>
              <option value="unless-stopped">unless-stopped</option>
              <option value="on-failure">on-failure</option>
            </select>
          </div>
        </div>

        <div className="p-4 border-t border-gray-700 flex gap-2">
          <button
            onClick={handleCreate}
            disabled={creating || !name || !image}
            className="flex-1 px-4 py-3 bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors disabled:opacity-50"
          >
            {creating ? '⏳ Creating...' : '🚀 Create & Start'}
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
