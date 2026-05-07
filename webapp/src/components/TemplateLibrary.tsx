import React, { useState, useEffect } from 'react';

interface Template {
  id: string;
  name: string;
  description: string;
  image: string;
  ports: string[];
  volumes: string[];
  env: Record<string, string>;
  network: string;
  restart_policy: string;
  created_by: string;
  created_at: string;
  public: boolean;
  usage_count: number;
  tags: string[];
}

interface TemplateLibraryProps {
  onClose: () => void;
  getAuthHeaders: () => Record<string, string>;
}

const TemplateLibrary: React.FC<TemplateLibraryProps> = ({ onClose, getAuthHeaders }) => {
  const [activeTab, setActiveTab] = useState<'browse' | 'create'>('browse');
  const [templates, setTemplates] = useState<Template[]>([]);
  const [loading, setLoading] = useState(false);
  const [deploying, setDeploying] = useState<string | null>(null);
  
  // Create form state
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [image, setImage] = useState('');
  const [ports, setPorts] = useState('');
  const [volumes, setVolumes] = useState('');
  const [env, setEnv] = useState('');
  const [network, setNetwork] = useState('bridge');
  const [restartPolicy, setRestartPolicy] = useState('unless-stopped');
  const [isPublic, setIsPublic] = useState(false);
  const [tags, setTags] = useState('');
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    if (activeTab === 'browse') {
      fetchTemplates();
    }
  }, [activeTab]);

  const fetchTemplates = async () => {
    setLoading(true);
    try {
      const res = await fetch('/api/templates', { headers: getAuthHeaders() });
      const data = await res.json();
      if (data.success) {
        setTemplates(data.data || []);
      }
    } catch (err) {
      console.error('Failed to fetch templates:', err);
    } finally {
      setLoading(false);
    }
  };

  const createTemplate = async () => {
    if (!name || !image) {
      alert('Name and image are required');
      return;
    }

    setCreating(true);
    try {
      const template = {
        name,
        description,
        image,
        ports: ports.split(',').filter(p => p.trim()),
        volumes: volumes.split(',').filter(v => v.trim()),
        env: env.split(',').reduce((acc, pair) => {
          const [k, v] = pair.split('=');
          if (k && v) acc[k.trim()] = v.trim();
          return acc;
        }, {} as Record<string, string>),
        network,
        restart_policy: restartPolicy,
        public: isPublic,
        tags: tags.split(',').filter(t => t.trim()),
        created_by: 'user',
      };

      const res = await fetch('/api/templates', {
        method: 'POST',
        headers: { ...getAuthHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify(template),
      });

      const data = await res.json();
      if (data.success) {
        setName('');
        setDescription('');
        setImage('');
        setPorts('');
        setVolumes('');
        setEnv('');
        setTags('');
        setActiveTab('browse');
        fetchTemplates();
      }
    } catch (err) {
      console.error('Failed to create template:', err);
    } finally {
      setCreating(false);
    }
  };

  const deployTemplate = async (templateId: string) => {
    const containerName = prompt('Enter container name:');
    if (!containerName) return;

    setDeploying(templateId);
    try {
      const res = await fetch(`/api/templates/${templateId}/deploy`, {
        method: 'POST',
        headers: { ...getAuthHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: containerName }),
      });

      const data = await res.json();
      if (data.success) {
        alert('Container deployed successfully!');
        fetchTemplates();
      } else {
        alert(`Failed: ${data.error}`);
      }
    } catch (err) {
      console.error('Failed to deploy template:', err);
      alert('Failed to deploy template');
    } finally {
      setDeploying(null);
    }
  };

  const deleteTemplate = async (templateId: string) => {
    if (!confirm('Delete this template?')) return;

    try {
      const res = await fetch(`/api/templates/${templateId}?user_id=user`, {
        method: 'DELETE',
        headers: getAuthHeaders(),
      });

      const data = await res.json();
      if (data.success) {
        fetchTemplates();
      }
    } catch (err) {
      console.error('Failed to delete template:', err);
    }
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div className="bg-gray-800 rounded-lg w-full max-w-4xl max-h-[90vh] overflow-hidden flex flex-col">
        <div className="p-4 border-b border-gray-700 flex justify-between items-center">
          <h2 className="text-xl font-bold text-white">📦 Template Library</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-white text-2xl">×</button>
        </div>

        <div className="flex border-b border-gray-700">
          <button
            onClick={() => setActiveTab('browse')}
            className={`flex-1 py-3 px-4 font-medium ${
              activeTab === 'browse'
                ? 'bg-blue-600 text-white'
                : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
            }`}
          >
            Browse Templates
          </button>
          <button
            onClick={() => setActiveTab('create')}
            className={`flex-1 py-3 px-4 font-medium ${
              activeTab === 'create'
                ? 'bg-blue-600 text-white'
                : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
            }`}
          >
            Create Template
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4">
          {activeTab === 'browse' ? (
            loading ? (
              <div className="text-center py-8 text-gray-400">Loading...</div>
            ) : templates.length === 0 ? (
              <div className="text-center py-8 text-gray-400">No templates yet. Create one!</div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {templates.map((template) => (
                  <div key={template.id} className="bg-gray-700 rounded-lg p-4">
                    <div className="flex items-start justify-between mb-2">
                      <div className="flex-1">
                        <h3 className="text-white font-bold">{template.name}</h3>
                        <p className="text-gray-400 text-sm">{template.description}</p>
                      </div>
                      {template.public && (
                        <span className="bg-green-600 text-white text-xs px-2 py-1 rounded">Public</span>
                      )}
                    </div>
                    
                    <div className="text-gray-300 text-sm mb-3 space-y-1">
                      <div>🐳 {template.image}</div>
                      {template.ports.length > 0 && (
                        <div>🔌 {template.ports.join(', ')}</div>
                      )}
                      <div>📊 Used {template.usage_count} times</div>
                    </div>

                    {template.tags.length > 0 && (
                      <div className="flex flex-wrap gap-1 mb-3">
                        {template.tags.map((tag, i) => (
                          <span key={i} className="bg-gray-600 text-gray-300 text-xs px-2 py-1 rounded">
                            {tag}
                          </span>
                        ))}
                      </div>
                    )}

                    <div className="flex gap-2">
                      <button
                        onClick={() => deployTemplate(template.id)}
                        disabled={deploying === template.id}
                        className="flex-1 bg-blue-600 hover:bg-blue-700 text-white px-3 py-2 rounded disabled:opacity-50"
                      >
                        {deploying === template.id ? 'Deploying...' : '🚀 Deploy'}
                      </button>
                      <button
                        onClick={() => deleteTemplate(template.id)}
                        className="bg-red-600 hover:bg-red-700 text-white px-3 py-2 rounded"
                      >
                        🗑️
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )
          ) : (
            <div className="space-y-4">
              <div>
                <label className="block text-gray-300 mb-1">Name *</label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="w-full bg-gray-700 text-white px-3 py-2 rounded"
                  placeholder="My Template"
                />
              </div>

              <div>
                <label className="block text-gray-300 mb-1">Description</label>
                <input
                  type="text"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  className="w-full bg-gray-700 text-white px-3 py-2 rounded"
                  placeholder="What does this template do?"
                />
              </div>

              <div>
                <label className="block text-gray-300 mb-1">Image *</label>
                <input
                  type="text"
                  value={image}
                  onChange={(e) => setImage(e.target.value)}
                  className="w-full bg-gray-700 text-white px-3 py-2 rounded"
                  placeholder="nginx:latest"
                />
              </div>

              <div>
                <label className="block text-gray-300 mb-1">Ports (comma-separated)</label>
                <input
                  type="text"
                  value={ports}
                  onChange={(e) => setPorts(e.target.value)}
                  className="w-full bg-gray-700 text-white px-3 py-2 rounded"
                  placeholder="8080:80,3000:3000"
                />
              </div>

              <div>
                <label className="block text-gray-300 mb-1">Volumes (comma-separated)</label>
                <input
                  type="text"
                  value={volumes}
                  onChange={(e) => setVolumes(e.target.value)}
                  className="w-full bg-gray-700 text-white px-3 py-2 rounded"
                  placeholder="/host/path:/container/path"
                />
              </div>

              <div>
                <label className="block text-gray-300 mb-1">Environment Variables</label>
                <input
                  type="text"
                  value={env}
                  onChange={(e) => setEnv(e.target.value)}
                  className="w-full bg-gray-700 text-white px-3 py-2 rounded"
                  placeholder="KEY=value,KEY2=value2"
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-gray-300 mb-1">Network</label>
                  <select
                    value={network}
                    onChange={(e) => setNetwork(e.target.value)}
                    className="w-full bg-gray-700 text-white px-3 py-2 rounded"
                  >
                    <option value="bridge">Bridge</option>
                    <option value="host">Host</option>
                    <option value="none">None</option>
                  </select>
                </div>

                <div>
                  <label className="block text-gray-300 mb-1">Restart Policy</label>
                  <select
                    value={restartPolicy}
                    onChange={(e) => setRestartPolicy(e.target.value)}
                    className="w-full bg-gray-700 text-white px-3 py-2 rounded"
                  >
                    <option value="no">No</option>
                    <option value="always">Always</option>
                    <option value="unless-stopped">Unless Stopped</option>
                    <option value="on-failure">On Failure</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-gray-300 mb-1">Tags (comma-separated)</label>
                <input
                  type="text"
                  value={tags}
                  onChange={(e) => setTags(e.target.value)}
                  className="w-full bg-gray-700 text-white px-3 py-2 rounded"
                  placeholder="web,nginx,production"
                />
              </div>

              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={isPublic}
                  onChange={(e) => setIsPublic(e.target.checked)}
                  className="w-4 h-4"
                />
                <label className="text-gray-300">Make this template public</label>
              </div>

              <button
                onClick={createTemplate}
                disabled={creating}
                className="w-full bg-blue-600 hover:bg-blue-700 text-white py-3 rounded font-medium disabled:opacity-50"
              >
                {creating ? 'Creating...' : '✨ Create Template'}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default TemplateLibrary;
