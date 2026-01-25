import { SuffixType } from './SuffixType.js';

export class SymbolIndexKey {
  private readonly packageNameValue: string;
  private readonly filePathValue: string;
  private readonly displayNameValue: string;
  private readonly suffixValue: SuffixType;
  private readonly fullDescriptorValue: string;

  constructor(
    packageName: string,
    filePath: string,
    displayName: string,
    suffix: SuffixType,
    fullDescriptor: string = displayName
  ) {
    this.packageNameValue = packageName;
    this.filePathValue = filePath;
    this.displayNameValue = displayName;
    this.suffixValue = suffix;
    this.fullDescriptorValue = fullDescriptor;
  }

  get packageName(): string {
    return this.packageNameValue;
  }

  get filePath(): string {
    return this.filePathValue;
  }

  get displayName(): string {
    return this.displayNameValue;
  }

  get suffix(): SuffixType {
    return this.suffixValue;
  }

  get fullDescriptor(): string {
    return this.fullDescriptorValue;
  }

  toString(): string {
    return `${this.packageNameValue}:${this.filePathValue}:${this.displayNameValue}`;
  }

  toFullKey(): string {
    return `${this.packageNameValue}:${this.filePathValue}:${this.fullDescriptorValue}`;
  }

  valueOf(): string {
    return this.toString();
  }

  equals(other: SymbolIndexKey): boolean {
    return (
      this.packageNameValue === other.packageNameValue &&
      this.filePathValue === other.filePathValue &&
      this.displayNameValue === other.displayNameValue &&
      this.suffixValue === other.suffixValue
    );
  }
}
