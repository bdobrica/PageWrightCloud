import React, { useState, useEffect, useRef } from 'react';
import { useParams } from 'react-router-dom';
import { Layout } from '../components/Layout';
import { VersionsList } from '../components/VersionsList';
import { ChatMessage } from '../components/ChatMessage';
import { FileAttachment } from '../components/FileAttachment';
import { apiClient } from '../api/client';
import { useWebSocket } from '../hooks/useWebSocket';
import type { JobStatusUpdate } from '../types/api';
import './Chat.css';

interface Message {
  id: string;
  text: string;
  sender: 'user' | 'agent';
  timestamp: Date;
}

export const Chat: React.FC = () => {
  const { fqdn } = useParams<{ fqdn: string }>();
  const [messages, setMessages] = useState<Message[]>([]);
  const [inputText, setInputText] = useState('');
  const [files, setFiles] = useState<File[]>([]);
  const [conversationId, setConversationId] = useState<string | undefined>();
  const [isLoading, setIsLoading] = useState(false);
  const [versionRefresh, setVersionRefresh] = useState(0);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const handleJobUpdate = (update: JobStatusUpdate) => {
    if (update.status === 'success') {
      setMessages((prev) => [
        ...prev,
        {
          id: Date.now().toString(),
          text: `✓ Build completed! Version ${update.build_id} is ready.`,
          sender: 'agent',
          timestamp: new Date(),
        },
      ]);
      setVersionRefresh((prev) => prev + 1);
    } else if (update.status === 'failed') {
      setMessages((prev) => [
        ...prev,
        {
          id: Date.now().toString(),
          text: `✗ Build failed: ${update.message || 'Unknown error'}`,
          sender: 'agent',
          timestamp: new Date(),
        },
      ]);
    }
  };

  useWebSocket(handleJobUpdate);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSend = async () => {
    if (!inputText.trim() && files.length === 0) return;

    const userMessage: Message = {
      id: Date.now().toString(),
      text: inputText,
      sender: 'user',
      timestamp: new Date(),
    };

    setMessages((prev) => [...prev, userMessage]);
    setIsLoading(true);

    try {
      const response = await apiClient.build(fqdn!, {
        message: inputText,
        conversation_id: conversationId,
        files,
      });

      if (response.question) {
        // Agent needs clarification
        setConversationId(response.conversation_id);
        setMessages((prev) => [
          ...prev,
          {
            id: Date.now().toString() + '-q',
            text: response.question || '',
            sender: 'agent',
            timestamp: new Date(),
          },
        ]);
      } else if (response.job_id) {
        // Job enqueued
        setConversationId(undefined);
        setMessages((prev) => [
          ...prev,
          {
            id: Date.now().toString() + '-j',
            text: 'Building your site... This may take a moment.',
            sender: 'agent',
            timestamp: new Date(),
          },
        ]);
      }

      setInputText('');
      setFiles([]);
    } catch (err: any) {
      setMessages((prev) => [
        ...prev,
        {
          id: Date.now().toString() + '-e',
          text: `Error: ${err.response?.data?.message || 'Failed to send message'}`,
          sender: 'agent',
          timestamp: new Date(),
        },
      ]);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Layout sidebar={<VersionsList fqdn={fqdn!} refresh={versionRefresh} />}>
      <div className="chat-container">
        <div className="chat-header">
          <h2>{fqdn}</h2>
          <div>
            <a href={`https://${fqdn}`} target="_blank" rel="noopener noreferrer" className="pure-button">
              View Live
            </a>
            <a href={`https://${fqdn}/preview`} target="_blank" rel="noopener noreferrer" className="pure-button">
              View Preview
            </a>
          </div>
        </div>

        <div className="chat-messages">
          {messages.length === 0 && (
            <div className="empty-chat">
              <p>Start building your site! Describe what you'd like to change.</p>
            </div>
          )}
          {messages.map((msg) => (
            <ChatMessage key={msg.id} message={msg} />
          ))}
          <div ref={messagesEndRef} />
        </div>

        <div className="chat-input">
          <FileAttachment files={files} onFilesChange={setFiles} disabled={isLoading} />
          <textarea
            value={inputText}
            onChange={(e) => setInputText(e.target.value)}
            onKeyPress={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleSend();
              }
            }}
            placeholder="Describe what you'd like to change..."
            disabled={isLoading}
            rows={3}
          />
          <button
            onClick={handleSend}
            disabled={isLoading || (!inputText.trim() && files.length === 0)}
            className="pure-button pure-button-primary"
          >
            {isLoading ? 'Sending...' : 'Send'}
          </button>
        </div>
      </div>
    </Layout>
  );
};
