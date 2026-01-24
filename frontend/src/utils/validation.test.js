import { validateURL, matchesPattern, validateHeaders } from './validation';

describe('Validation Utilities', () => {
  describe('validateURL', () => {
    test('should validate correct HTTP URLs', () => {
      expect(validateURL('http://example.com')).toBe(true);
      expect(validateURL('http://localhost:3000')).toBe(true);
    });

    test('should validate correct HTTPS URLs', () => {
      expect(validateURL('https://example.com')).toBe(true);
    });

    test('should validate correct SOCKS5 URLs', () => {
      expect(validateURL('socks5://localhost:9050')).toBe(true);
    });

    test('should reject invalid protocols', () => {
      expect(validateURL('ftp://example.com')).toBe(false);
      expect(validateURL('gopher://example.com')).toBe(false);
    });

    test('should reject malformed URLs', () => {
      expect(validateURL('not-a-url')).toBe(false);
      expect(validateURL('')).toBe(false);
      expect(validateURL(null)).toBe(false);
    });
  });

  describe('matchesPattern', () => {
    test('should match exact strings', () => {
      expect(matchesPattern('gpt-4', 'gpt-4')).toBe(true);
      expect(matchesPattern('gpt-4', 'gpt-3.5')).toBe(false);
    });

    test('should match wildcard (*)', () => {
      expect(matchesPattern('anything', '*')).toBe(true);
      expect(matchesPattern('', '*')).toBe(false);
    });

    test('should match prefix patterns (prefix-*)', () => {
      expect(matchesPattern('gpt-4', 'gpt-*')).toBe(true);
      expect(matchesPattern('gpt-3.5', 'gpt-*')).toBe(true);
      expect(matchesPattern('claude-3', 'gpt-*')).toBe(false);
    });

    test('should match suffix patterns (*-suffix)', () => {
      expect(matchesPattern('gpt-4-turbo', '*-turbo')).toBe(true);
      expect(matchesPattern('claude-3-turbo', '*-turbo')).toBe(true);
      expect(matchesPattern('gpt-4', '*-turbo')).toBe(false);
    });

    test('should match substring patterns (*substring*)', () => {
      expect(matchesPattern('gpt-4-preview', '*4*')).toBe(true);
      expect(matchesPattern('claude-3-opus', '*opus*')).toBe(true);
      expect(matchesPattern('gpt-3.5', '*4*')).toBe(false);
    });
    
    test('should handle edge cases', () => {
        expect(matchesPattern(null, '*')).toBe(false);
        expect(matchesPattern('model', null)).toBe(false);
    });
  });

  describe('validateHeaders', () => {
    test('should validate valid headers', () => {
      const headers = {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer token'
      };
      expect(validateHeaders(headers)).toBe(true);
    });

    test('should return true for empty headers', () => {
      expect(validateHeaders({})).toBe(true);
      expect(validateHeaders(null)).toBe(true);
    });

    test('should reject headers with empty keys', () => {
      const headers = {
        '': 'value'
      };
      expect(validateHeaders(headers)).toBe(false);
    });

    test('should reject headers with whitespace-only keys', () => {
        const headers = {
          '   ': 'value'
        };
        expect(validateHeaders(headers)).toBe(false);
      });
  });
});
