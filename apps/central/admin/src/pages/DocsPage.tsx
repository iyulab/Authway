import React, { useState, useEffect } from 'react'
import {
  BookOpenIcon,
  MagnifyingGlassIcon,
  DocumentTextIcon,
  FolderIcon,
  ChevronRightIcon,
  ChevronDownIcon,
  ArrowDownTrayIcon,
} from '@heroicons/react/24/outline'
import { toast } from 'react-hot-toast'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'

// Import Prism for syntax highlighting
import Prism from 'prismjs'
import 'prismjs/themes/prism-tomorrow.css'
// Import common languages
import 'prismjs/components/prism-javascript'
import 'prismjs/components/prism-typescript'
import 'prismjs/components/prism-jsx'
import 'prismjs/components/prism-tsx'
import 'prismjs/components/prism-css'
import 'prismjs/components/prism-bash'
import 'prismjs/components/prism-json'
import 'prismjs/components/prism-markdown'
import 'prismjs/components/prism-python'
import 'prismjs/components/prism-go'
import 'prismjs/components/prism-sql'
import 'prismjs/components/prism-yaml'
import 'prismjs/components/prism-docker'
import 'prismjs/components/prism-csharp'

interface DocFile {
  path: string
  name: string
  type: 'file' | 'folder'
  children?: DocFile[]
}

interface DocIndex {
  files: DocFile[]
}

const DocsPage: React.FC = () => {
  const [docIndex, setDocIndex] = useState<DocFile[]>([])
  const [selectedDoc, setSelectedDoc] = useState<{ path: string; content: string; name: string } | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<DocFile[]>([])
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set(['quickstart', 'features', 'deployment']))
  const [loading, setLoading] = useState(true)

  // Load documentation index
  useEffect(() => {
    loadDocIndex()
  }, [])

  const loadDocIndex = async () => {
    try {
      setLoading(true)
      const response = await fetch('/docs/index.json')
      if (!response.ok) {
        throw new Error('Failed to load doc index')
      }
      const data: DocIndex = await response.json()
      setDocIndex(data.files)
    } catch (error) {
      console.error('Failed to load docs index:', error)
      toast.error('문서 목록을 불러올 수 없습니다')
    } finally {
      setLoading(false)
    }
  }

  const loadDocContent = async (path: string, name: string) => {
    try {
      const response = await fetch(`/docs/${path}`)
      if (!response.ok) {
        throw new Error('Failed to load document')
      }
      const content = await response.text()
      setSelectedDoc({ path, content, name })
      // Trigger Prism highlighting after content loads
      setTimeout(() => Prism.highlightAll(), 100)
    } catch (error) {
      console.error('Failed to load doc:', error)
      toast.error('문서를 불러올 수 없습니다')
    }
  }

  const handleSearch = () => {
    if (!searchQuery.trim()) {
      setSearchResults([])
      return
    }

    const results: DocFile[] = []
    const searchLower = searchQuery.toLowerCase()

    const searchInFiles = (files: DocFile[]) => {
      files.forEach(file => {
        if (file.type === 'file' && file.name.toLowerCase().includes(searchLower)) {
          results.push(file)
        }
        if (file.children) {
          searchInFiles(file.children)
        }
      })
    }

    searchInFiles(docIndex)
    setSearchResults(results)

    if (results.length === 0) {
      toast.error('검색 결과가 없습니다')
    }
  }

  const toggleFolder = (path: string) => {
    const newExpanded = new Set(expandedFolders)
    if (newExpanded.has(path)) {
      newExpanded.delete(path)
    } else {
      newExpanded.add(path)
    }
    setExpandedFolders(newExpanded)
  }

  const downloadDoc = (path: string, name: string) => {
    const link = document.createElement('a')
    link.href = `/docs/${path}`
    link.download = name
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
  }

  // Render file tree recursively
  const renderFileTree = (files: DocFile[], depth = 0) => {
    return files.map((file) => (
      <div key={file.path} style={{ marginLeft: `${depth * 16}px` }}>
        {file.type === 'folder' ? (
          <div>
            <button
              onClick={() => toggleFolder(file.path)}
              className="flex items-center space-x-2 px-3 py-2 hover:bg-gray-100 rounded w-full text-left"
            >
              {expandedFolders.has(file.path) ? (
                <ChevronDownIcon className="h-4 w-4 text-gray-500" />
              ) : (
                <ChevronRightIcon className="h-4 w-4 text-gray-500" />
              )}
              <FolderIcon className="h-5 w-5 text-yellow-500" />
              <span className="text-sm font-medium text-gray-700">{file.name}</span>
            </button>
            {expandedFolders.has(file.path) && file.children && (
              <div className="mt-1">
                {renderFileTree(file.children, depth + 1)}
              </div>
            )}
          </div>
        ) : (
          <button
            onClick={() => loadDocContent(file.path, file.name)}
            className={`flex items-center space-x-2 px-3 py-2 hover:bg-gray-100 rounded w-full text-left ${
              selectedDoc?.path === file.path ? 'bg-indigo-50' : ''
            }`}
          >
            <DocumentTextIcon className="h-5 w-5 text-gray-400" />
            <span className="text-sm text-gray-700">{file.name}</span>
          </button>
        )}
      </div>
    ))
  }

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="bg-gradient-to-r from-indigo-600 to-purple-600 px-6 py-3 text-white shadow-md">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <BookOpenIcon className="h-7 w-7" />
            <div>
              <h1 className="text-xl font-bold">개발자 가이드</h1>
              <p className="text-xs text-indigo-100">Authway API 문서 및 통합 가이드</p>
            </div>
          </div>

          {/* Search */}
          <div className="flex items-center space-x-2">
            <div className="relative">
              <input
                type="text"
                placeholder="문서 검색..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
                className="pl-10 pr-4 py-2 rounded-lg bg-white/10 border border-white/20 text-white placeholder-white/60 focus:outline-none focus:ring-2 focus:ring-white/30 w-64"
              />
              <MagnifyingGlassIcon className="h-5 w-5 absolute left-3 top-2.5 text-white/60" />
            </div>
            <button
              onClick={handleSearch}
              className="px-4 py-2 bg-white/20 hover:bg-white/30 rounded-lg transition-colors"
            >
              검색
            </button>
          </div>
        </div>
      </div>

      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar - File Tree */}
        <div className="w-64 bg-white border-r border-gray-200 overflow-y-auto flex-shrink-0">
          <div className="p-3">
            <h2 className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">
              문서 목록
            </h2>
            {loading ? (
              <div className="text-center py-8 text-gray-500">
                로딩 중...
              </div>
            ) : docIndex.length > 0 ? (
              renderFileTree(docIndex)
            ) : (
              <div className="text-center py-8 text-gray-500">
                문서가 없습니다
              </div>
            )}
          </div>

          {/* Search Results */}
          {searchResults.length > 0 && (
            <div className="border-t border-gray-200 p-3">
              <h2 className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">
                검색 결과 ({searchResults.length})
              </h2>
              {searchResults.map((result) => (
                <button
                  key={result.path}
                  onClick={() => loadDocContent(result.path, result.name)}
                  className="flex items-center space-x-2 px-3 py-2 hover:bg-gray-100 rounded w-full text-left mb-1"
                >
                  <DocumentTextIcon className="h-5 w-5 text-gray-400" />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-gray-700 truncate">{result.name}</p>
                    <p className="text-xs text-gray-500 truncate">{result.path}</p>
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Main Content - Document Viewer */}
        <div className="flex-1 overflow-y-auto bg-gray-50">
          {selectedDoc ? (
            <div className="h-full">
              {/* Document Header */}
              <div className="bg-white border-b border-gray-200 px-6 py-4 sticky top-0 z-10">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <h1 className="text-xl font-bold text-gray-900 mb-1">
                      {selectedDoc.name}
                    </h1>
                    <div className="flex items-center space-x-4 text-xs text-gray-500">
                      <span>{selectedDoc.path}</span>
                    </div>
                  </div>
                  <div className="flex items-center space-x-2">
                    <button
                      onClick={() => downloadDoc(selectedDoc.path, selectedDoc.name)}
                      className="p-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded transition-colors"
                      title="다운로드"
                    >
                      <ArrowDownTrayIcon className="h-5 w-5" />
                    </button>
                  </div>
                </div>
              </div>

              {/* Markdown Content */}
              <div className="px-6 py-6">
                <div className="bg-white rounded-lg shadow-sm p-8 prose prose-indigo prose-lg max-w-none">
                  <ReactMarkdown
                    remarkPlugins={[remarkGfm]}
                    rehypePlugins={[rehypeRaw]}
                  >
                    {selectedDoc.content}
                  </ReactMarkdown>
                </div>
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-center h-full">
              <div className="text-center">
                <BookOpenIcon className="h-16 w-16 text-gray-300 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-gray-900 mb-2">문서를 선택하세요</h3>
                <p className="text-gray-500">
                  왼쪽 목록에서 문서를 선택하여 내용을 확인할 수 있습니다
                </p>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export default DocsPage
