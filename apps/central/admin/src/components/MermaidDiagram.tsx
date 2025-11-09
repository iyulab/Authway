import React, { useEffect, useRef, useState } from 'react';
import mermaid from 'mermaid';

interface MermaidDiagramProps {
  chart: string;
  className?: string;
}

// Initialize mermaid with custom theme
mermaid.initialize({
  startOnLoad: false,
  theme: 'default',
  securityLevel: 'loose',
  themeVariables: {
    primaryColor: '#3b82f6',
    primaryTextColor: '#fff',
    primaryBorderColor: '#2563eb',
    lineColor: '#6b7280',
    secondaryColor: '#10b981',
    tertiaryColor: '#f59e0b',
  },
});

let idCounter = 0;

export const MermaidDiagram: React.FC<MermaidDiagramProps> = ({ chart, className = '' }) => {
  const elementRef = useRef<HTMLDivElement>(null);
  const [id] = useState(() => `mermaid-${++idCounter}`);
  const [svg, setSvg] = useState<string>('');

  useEffect(() => {
    const renderDiagram = async () => {
      if (!chart) return;

      try {
        // Remove the element if it exists to avoid ID conflicts
        const existingElement = document.getElementById(id);
        if (existingElement) {
          existingElement.remove();
        }

        // Render the diagram
        const { svg: renderedSvg } = await mermaid.render(id, chart);
        setSvg(renderedSvg);
      } catch (error) {
        console.error('Mermaid rendering error:', error);
        setSvg(`<div class="text-red-500 p-4">Error rendering diagram: ${error}</div>`);
      }
    };

    renderDiagram();
  }, [chart, id]);

  return (
    <div
      ref={elementRef}
      className={className}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
};

export default MermaidDiagram;
