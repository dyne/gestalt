import { SuffixType } from './SuffixType.js';

export interface ParsedSymbol {
  packageName: string;
  filePath: string;
  displayName: string;
  suffix: SuffixType;
  fullDescriptor: string;
}

export class SymbolParser {
  parse(symbol: string): ParsedSymbol {
    return {
      packageName: this.extractPackageName(symbol),
      filePath: this.extractFilePath(symbol),
      displayName: this.extractDisplayName(symbol),
      suffix: this.detectSuffixType(symbol),
      fullDescriptor: this.extractFullDescriptor(symbol),
    };
  }

  private extractPackageName(symbol: string): string {
    const parts = symbol.split(' ');
    return parts.length >= 3 ? parts[2] : '';
  }

  private extractDisplayName(symbol: string): string {
    const lastSpaceIndex = symbol.lastIndexOf(' ');
    if (lastSpaceIndex === -1) {
      return '';
    }

    const descriptorPart = symbol.slice(lastSpaceIndex + 1);

    if (descriptorPart.endsWith('/')) {
      const descriptor = descriptorPart.slice(0, -1);
      const leafName = this.extractLeafName(descriptor);
      return leafName;
    }

    const lastSlashInPath = descriptorPart.lastIndexOf('/');
    if (lastSlashInPath === -1) {
      return '';
    }

    const descriptor = descriptorPart.slice(lastSlashInPath + 1);
    if (descriptor.length === 0) {
      return '';
    }

    const leafName = this.extractLeafName(descriptor);
    if (leafName.length === 0) {
      return '';
    }

    if (leafName.endsWith('().')) {
      return leafName.slice(0, -3);
    }

    const suffixChar = leafName[leafName.length - 1];
    if (suffixChar === '#' || suffixChar === '.' || suffixChar === '/') {
      return leafName.slice(0, -1);
    }

    return leafName;
  }

  private extractLeafName(descriptor: string): string {
    const lastHashIndex = descriptor.lastIndexOf('#');
    const lastSlashIndex = descriptor.lastIndexOf('/');

    if (lastHashIndex !== -1 && lastHashIndex >= lastSlashIndex) {
      if (lastHashIndex < descriptor.length - 1) {
        return descriptor.slice(lastHashIndex + 1);
      }
      return descriptor;
    }

    if (lastSlashIndex !== -1) {
      if (lastSlashIndex < descriptor.length - 1) {
        return descriptor.slice(lastSlashIndex + 1);
      }
      return descriptor;
    }

    return descriptor;
  }

  private extractFilePath(symbol: string): string {
    const match = symbol.match(/(\S+\/)(?:\\+)?`([^`\\]+)(?:\\+)?`/);
    return match ? match[1] + match[2] : '';
  }

  private extractFullDescriptor(symbol: string): string {
    const lastSpaceIndex = symbol.lastIndexOf(' ');
    if (lastSpaceIndex === -1) {
      return '';
    }

    const descriptorPart = symbol.slice(lastSpaceIndex + 1);
    const lastSlashInPath = descriptorPart.lastIndexOf('/');
    if (lastSlashInPath === -1) {
      return '';
    }

    return descriptorPart.slice(lastSlashInPath + 1);
  }

  private detectSuffixType(symbol: string): SuffixType {
    const lastSpaceIndex = symbol.lastIndexOf(' ');
    if (lastSpaceIndex === -1) {
      return SuffixType.Namespace;
    }

    const descriptorPart = symbol.slice(lastSpaceIndex + 1);

    if (descriptorPart.endsWith('().')) {
      return SuffixType.Method;
    }
    if (descriptorPart.endsWith('#')) {
      return SuffixType.Type;
    }
    if (descriptorPart.endsWith('.')) {
      return SuffixType.Term;
    }
    if (descriptorPart.endsWith('/')) {
      return SuffixType.Namespace;
    }

    return SuffixType.Term;
  }
}
